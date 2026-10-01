package mcpsrv

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const databaseMaxRows = 200
const databaseORMMarker = "ODOOCTL_MCP_RESULT="
const databaseORMOutputLimit = 4 << 20

var databaseIdentifier = regexp.MustCompile(`^[a-z_][a-z0-9_]*$`)
var databaseModel = regexp.MustCompile(`^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)*$`)

// Inspection deliberately excludes secret-bearing columns, even when DBA grants allow them.
func databaseSensitive(name string) bool {
	name = strings.ToLower(name)
	for _, part := range []string{"password", "passwd", "secret", "token", "credential", "api_key", "apikey", "private_key", "privatekey", "session", "cookie", "otp", "totp", "signature", "attachment", "datas", "dbfilter", "config_parameter", "smtp_pass", "passphrase", "access_key", "encryption"} {
		if strings.Contains(name, part) {
			return true
		}
	}
	return false
}

func databaseRole(role string) error {
	if !databaseIdentifier.MatchString(role) || len(role) > 63 {
		return fmt.Errorf("an explicit dedicated SQLRole is required (lowercase PostgreSQL identifier)")
	}
	if role == "odoo" || role == "postgres" || role == "root" || role == "admin" || role == "administrator" || role == "superuser" || strings.HasPrefix(role, "pg_") {
		return fmt.Errorf("SQLRole must not be a privileged or application role")
	}
	return nil
}

func databaseTables(tables []string) error {
	if len(tables) == 0 || len(tables) > 50 {
		return fmt.Errorf("SQLTables must explicitly allow 1 to 50 public tables")
	}
	for _, table := range tables {
		if !databaseIdentifier.MatchString(table) || len(table) > 63 || databaseSensitive(table) || strings.HasPrefix(table, "pg_") || strings.HasPrefix(table, "sql_") {
			return fmt.Errorf("invalid or sensitive allowed table %q", table)
		}
	}
	return nil
}

func databaseContains(items []string, value string) bool {
	for _, item := range items {
		if item == value {
			return true
		}
	}
	return false
}

// SQL is a small language, not arbitrary PostgreSQL: explicit columns, one public
// table, scalar comparisons joined by AND/OR, and optional column ordering.
// In particular there are no functions, casts, joins, subqueries, aliases or stars.
func databaseSQLTokens(query string) ([]string, error) {
	if len(query) == 0 || len(query) > 8192 {
		return nil, fmt.Errorf("query must contain 1 to 8192 bytes")
	}
	var tokens []string
	for i := 0; i < len(query); {
		c := query[i]
		if c == ' ' || c == '\t' || c == '\n' || c == '\r' {
			i++
			continue
		}
		start := i
		switch {
		case c == '\'':
			i++
			closed := false
			for i < len(query) {
				if query[i] == '\\' || query[i] < 32 || query[i] > 126 {
					return nil, fmt.Errorf("string constants must be printable ASCII without backslashes")
				}
				if query[i] == '\'' {
					i++
					if i < len(query) && query[i] == '\'' {
						i++
						continue
					}
					closed = true
					break
				}
				i++
			}
			if !closed {
				return nil, fmt.Errorf("unterminated string constant")
			}
		case c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c == '_':
			i++
			for i < len(query) && (query[i] >= 'a' && query[i] <= 'z' || query[i] >= 'A' && query[i] <= 'Z' || query[i] >= '0' && query[i] <= '9' || query[i] == '_') {
				i++
			}
		case c >= '0' && c <= '9':
			i++
			for i < len(query) && (query[i] >= '0' && query[i] <= '9' || query[i] == '.') {
				i++
			}
			if _, err := strconv.ParseFloat(query[start:i], 64); err != nil {
				return nil, fmt.Errorf("invalid numeric literal")
			}
		case c == ',' || c == '=':
			i++
		case c == '<' || c == '>':
			i++
			if i < len(query) && (query[i] == '=' || c == '<' && query[i] == '>') {
				i++
			}
		default:
			return nil, fmt.Errorf("unsupported SQL syntax at byte %d", i)
		}
		token := query[start:i]
		if token[0] != '\'' {
			token = strings.ToLower(token)
		}
		tokens = append(tokens, token)
		if len(tokens) > 512 {
			return nil, fmt.Errorf("too many SQL tokens")
		}
	}
	return tokens, nil
}

type databaseSelect struct {
	SQL     string
	Table   string
	Columns []string
}

func databaseValidateSQL(query string, tables []string) (databaseSelect, error) {
	var result databaseSelect
	if err := databaseTables(tables); err != nil {
		return result, err
	}
	tokens, err := databaseSQLTokens(query)
	if err != nil {
		return result, err
	}
	i := 0
	take := func(value string) bool {
		if i < len(tokens) && tokens[i] == value {
			i++
			return true
		}
		return false
	}
	column := func() bool {
		if i >= len(tokens) || !databaseIdentifier.MatchString(tokens[i]) || databaseSensitive(tokens[i]) {
			return false
		}
		// Keywords cannot become identifiers in this deliberately restricted grammar.
		if databaseContains([]string{"select", "from", "where", "order", "by", "and", "or", "null", "true", "false", "asc", "desc", "is", "not", "limit", "offset", "union", "current_user", "session_user", "current_role", "current_catalog", "current_schema", "current_date", "current_time", "current_timestamp", "localtime", "localtimestamp", "user"}, tokens[i]) {
			return false
		}
		result.Columns = append(result.Columns, tokens[i])
		i++
		return len(result.Columns) <= 64
	}
	invalid := func() (databaseSelect, error) {
		return databaseSelect{}, fmt.Errorf("only SELECT explicit_columns FROM allowed_table [WHERE column comparison literal [AND/OR ...]] [ORDER BY column [ASC/DESC], ...] is supported")
	}
	if !take("select") || !column() {
		return invalid()
	}
	for take(",") {
		if !column() {
			return invalid()
		}
	}
	if !take("from") || i >= len(tokens) || !databaseContains(tables, tokens[i]) {
		return invalid()
	}
	result.Table = tokens[i]
	tokens[i] = "ONLY public." + result.Table
	i++
	if take("where") {
		for {
			if !column() {
				return invalid()
			}
			if take("is") {
				take("not")
				if !take("null") {
					return invalid()
				}
			} else {
				if i >= len(tokens) || !databaseContains([]string{"=", "<>", "<", ">", "<=", ">="}, tokens[i]) {
					return invalid()
				}
				i++
				if i >= len(tokens) {
					return invalid()
				}
				v := tokens[i]
				if v[0] != '\'' && !(v[0] >= '0' && v[0] <= '9') && v != "true" && v != "false" {
					return invalid()
				}
				i++
			}
			if !take("and") && !take("or") {
				break
			}
		}
	}
	if take("order") {
		if !take("by") {
			return invalid()
		}
		for {
			if !column() {
				return invalid()
			}
			if !take("asc") {
				take("desc")
			}
			if !take(",") {
				break
			}
		}
	}
	if i != len(tokens) {
		return invalid()
	}
	result.SQL = strings.Join(tokens, " ") + " LIMIT 201"
	return result, nil
}

func databaseSQLString(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

func databaseSQLScript(query string, columns []string, table string, tables []string) string {
	quotedTables := make([]string, len(tables))
	for i, name := range tables {
		quotedTables[i] = databaseSQLString(name)
	}
	// Owners (including inherited owner privileges) can bypass RLS. Reject them
	// for the entire allowlist, even for schema inspection and FORCE RLS tables.
	ownerGuard := fmt.Sprintf(`DO $guard$ BEGIN
 IF EXISTS (SELECT 1 FROM pg_catalog.pg_class c JOIN pg_catalog.pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname='public' AND c.relname IN (%s) AND pg_catalog.pg_has_role(current_user,c.relowner,'USAGE')) THEN RAISE EXCEPTION 'SQL role must not have allowed table owner privileges'; END IF;
END $guard$;
`, strings.Join(quotedTables, ","))
	guard := ""
	if table != "" {
		quoted := make([]string, len(columns))
		for i, name := range columns {
			quoted[i] = databaseSQLString(name)
		}
		// Only ordinary tables and built-in scalar column types: user-defined types
		// can execute custom input/output/comparison code, even in read-only sessions.
		guard = fmt.Sprintf(`DO $guard$ BEGIN
 IF NOT EXISTS (SELECT 1 FROM pg_catalog.pg_class c JOIN pg_catalog.pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname='public' AND c.relname=%s AND c.relkind='r') THEN RAISE EXCEPTION 'ordinary public table required'; END IF;
 IF EXISTS (SELECT 1 FROM unnest(ARRAY[%s]) requested(name) WHERE NOT EXISTS (SELECT 1 FROM pg_catalog.pg_attribute a JOIN pg_catalog.pg_class c ON c.oid=a.attrelid JOIN pg_catalog.pg_namespace n ON n.oid=c.relnamespace JOIN pg_catalog.pg_type t ON t.oid=a.atttypid JOIN pg_catalog.pg_namespace tn ON tn.oid=t.typnamespace WHERE n.nspname='public' AND c.relname=%s AND a.attname=requested.name AND a.attnum>0 AND NOT a.attisdropped AND a.attgenerated='' AND tn.nspname='pg_catalog' AND t.typname IN ('int2','int4','int8','float4','float8','numeric','bool','text','varchar','bpchar','date','timestamp','timestamptz'))) THEN RAISE EXCEPTION 'unsupported or inaccessible column'; END IF;
END $guard$;
`, databaseSQLString(table), strings.Join(quoted, ","), databaseSQLString(table))
	}
	return `BEGIN READ ONLY;
SET LOCAL statement_timeout = '5s';
SET LOCAL lock_timeout = '1s';
SET LOCAL idle_in_transaction_session_timeout = '10s';
SET LOCAL search_path = pg_catalog;
SET LOCAL standard_conforming_strings = on;
SET LOCAL row_security = on;
SAVEPOINT inspection;
DO $guard$ BEGIN
 IF current_setting('transaction_read_only') <> 'on' THEN RAISE EXCEPTION 'read-only transaction required'; END IF;
 IF NOT EXISTS (SELECT 1 FROM pg_catalog.pg_roles WHERE rolname=current_user AND NOT rolsuper AND NOT rolcreaterole AND NOT rolcreatedb AND NOT rolreplication AND NOT rolbypassrls) OR current_user <> session_user THEN RAISE EXCEPTION 'dedicated unprivileged SQL role required'; END IF;
 IF EXISTS (SELECT 1 FROM pg_catalog.pg_roles r WHERE (r.rolsuper OR r.rolbypassrls OR r.rolname IN ('odoo','postgres')) AND pg_catalog.pg_has_role(current_user,r.oid,'USAGE')) THEN RAISE EXCEPTION 'SQL role must not inherit privileged or application roles'; END IF;
END $guard$;
` + ownerGuard + guard + "SELECT COALESCE(json_agg(row_to_json(result)), '[]'::json) FROM (" + query + ") result;\nROLLBACK;\n"
}

type databaseQueryInput struct {
	Query string `json:"query" jsonschema:"Restricted SELECT over explicit columns of one configured public table; no functions, joins, stars, comments, or transaction controls"`
}

type databaseSchemaInput struct {
	Table string `json:"table,omitempty" jsonschema:"Optional configured public table"`
}

type databaseORMInput struct {
	Model      string   `json:"model"`
	Fields     []string `json:"fields,omitempty"`
	Domain     []any    `json:"domain,omitempty"`
	Order      string   `json:"order,omitempty"`
	GroupBy    []string `json:"group_by,omitempty"`
	Limit      int      `json:"limit,omitempty"`
	Offset     int      `json:"offset,omitempty"`
	CompanyIDs []int    `json:"company_ids,omitempty"`
}

func databaseValidateORM(in databaseORMInput, operation string, options Options) error {
	if options.OdooUserID <= 1 {
		return fmt.Errorf("an explicit non-superuser OdooUserID is required")
	}
	if len(options.CompanyIDs) > 50 {
		return fmt.Errorf("at most 50 configured companies are allowed")
	}
	for _, company := range options.CompanyIDs {
		if company <= 0 {
			return fmt.Errorf("configured company IDs must be positive")
		}
	}
	if len(options.Models) == 0 || !databaseModel.MatchString(in.Model) || !databaseContains(options.Models, in.Model) || databaseSensitive(in.Model) {
		return fmt.Errorf("model must be explicitly configured in Models and non-sensitive")
	}
	if in.Limit < 0 || in.Limit > databaseMaxRows || in.Offset < 0 || in.Offset > 10000 || len(in.Fields) > 64 || len(in.GroupBy) > 4 || len(in.Domain) > 64 || len(in.Order) > 512 || len(in.CompanyIDs) > 50 {
		return fmt.Errorf("ORM inspection bounds exceeded")
	}
	field := func(value string, aggregate bool) bool {
		parts := strings.Split(value, ":")
		return len(parts) <= 2 && databaseIdentifier.MatchString(parts[0]) && !databaseSensitive(parts[0]) && (len(parts) == 1 || aggregate && databaseContains([]string{"sum", "avg", "min", "max", "count", "count_distinct"}, parts[1]))
	}
	for _, name := range in.Fields {
		if !field(name, operation == "read_group") {
			return fmt.Errorf("invalid or sensitive field %q", name)
		}
	}
	for _, name := range in.GroupBy {
		if !field(name, false) {
			return fmt.Errorf("invalid or sensitive group field")
		}
	}
	if operation == "read_group" && (len(in.GroupBy) == 0 || len(in.Fields) == 0) {
		return fmt.Errorf("read_group requires explicit fields and group_by")
	}
	if operation != "read_group" && len(in.GroupBy) != 0 || operation == "model_info" && (len(in.Domain) != 0 || in.Order != "" || in.Offset != 0) {
		return fmt.Errorf("parameters do not apply to this operation")
	}
	if in.Order != "" {
		parts := strings.Split(in.Order, ",")
		if len(parts) > 8 {
			return fmt.Errorf("too many ordering fields")
		}
		for _, part := range parts {
			words := strings.Fields(part)
			if len(words) == 0 || len(words) > 2 || !field(words[0], false) || len(words) == 2 && words[1] != "asc" && words[1] != "desc" {
				return fmt.Errorf("order supports only stored field names with asc/desc")
			}
		}
	}
	for _, company := range in.CompanyIDs {
		if company <= 0 || !databaseContainsInt(options.CompanyIDs, company) {
			return fmt.Errorf("company must be explicitly configured in CompanyIDs")
		}
	}
	// Odoo domains are prefix Boolean expressions with implicit AND at the end.
	need := 0
	for _, node := range in.Domain {
		if op, ok := node.(string); ok {
			if op != "&" && op != "|" && op != "!" {
				return fmt.Errorf("unsupported domain operator")
			}
			if need == 0 {
				need = 1
			}
			if op != "!" {
				need++
			}
			continue
		}
		leaf, ok := node.([]any)
		if !ok || len(leaf) != 3 {
			return fmt.Errorf("domain leaves must be [field, operator, value]")
		}
		name, ok := leaf[0].(string)
		if !ok || !field(name, false) {
			return fmt.Errorf("invalid or sensitive domain field")
		}
		op, ok := leaf[1].(string)
		if !ok || !databaseContains([]string{"=", "!=", "<", ">", "<=", ">=", "in", "not in", "like", "ilike", "=like", "=ilike"}, op) {
			return fmt.Errorf("unsupported domain comparison")
		}
		values, list := leaf[2].([]any)
		if (op == "in" || op == "not in") != list || len(values) > 200 {
			return fmt.Errorf("domain membership requires a bounded scalar list")
		}
		if !list {
			values = []any{leaf[2]}
		}
		for _, value := range values {
			switch v := value.(type) {
			case nil, bool, float64, int:
			case string:
				if len(v) > 1024 {
					return fmt.Errorf("domain string too long")
				}
			default:
				return fmt.Errorf("domain values must be scalars")
			}
		}
		if need > 0 {
			need--
		}
	}
	if need != 0 {
		return fmt.Errorf("incomplete prefix domain")
	}
	return nil
}

func databaseContainsInt(items []int, value int) bool {
	for _, item := range items {
		if item == value {
			return true
		}
	}
	return false
}

// JSON is embedded as a quoted Python string, never as a Python object literal.
func databaseORMScript(in databaseORMInput, operation string, options Options) (string, error) {
	if err := databaseValidateORM(in, operation, options); err != nil {
		return "", err
	}
	if !databaseContains([]string{"model_info", "search_read", "read_group"}, operation) {
		return "", fmt.Errorf("unsupported ORM operation")
	}
	if in.Limit == 0 {
		in.Limit = databaseMaxRows
	}
	params, err := json.Marshal(map[string]any{"input": in, "operation": operation, "uid": options.OdooUserID, "companies": options.CompanyIDs})
	if err != nil {
		return "", err
	}
	// ASCII JSON string escapes are also valid Python string escapes. Escape single
	// quotes separately, and double backslashes to preserve JSON's own escapes.
	quoted := "'" + strings.ReplaceAll(strings.ReplaceAll(string(params), "\\", "\\\\"), "'", "\\'") + "'"
	return "import json\nfrom odoo import api\np = json.loads(" + quoted + ")\n" + databasePython, nil
}

const databasePython = `cr = env.cr
try:
    cr.rollback()
    # Psycopg implicitly begins here; a nested BEGIN READ ONLY is ineffective.
    cr.execute("SET TRANSACTION READ ONLY")
    cr.execute("SHOW transaction_read_only")
    read_only = cr.fetchone()[0] == 'on'
    if not read_only:
        raise ValueError('read-only transaction required')
    cr.execute("SET LOCAL statement_timeout = '5s'")
    cr.execute("SET LOCAL lock_timeout = '1s'")
    cr.execute("SET LOCAL idle_in_transaction_session_timeout = '10s'")
    cr.execute("SAVEPOINT inspection")
    env.clear()
    inspection = api.Environment(cr, p['uid'], {})
    if inspection.su or inspection.uid == 1:
        raise ValueError('non-superuser environment required')
    user = inspection.user
    if not user.exists() or not user.active:
        raise ValueError('active configured user required')
    allowed = user.company_ids.ids
    configured = p['companies'] or [user.company_id.id]
    if any(c not in allowed for c in configured):
        raise ValueError('configured company not allowed for user')
    a = p['input']
    companies = a.get('company_ids') or configured
    if any(c not in configured or c not in allowed for c in companies):
        raise ValueError('company not allowed')
    inspection = api.Environment(cr, p['uid'], {'allowed_company_ids': companies, 'active_test': True})
    model = inspection[a['model']]
    # Odoo 18+ uses check_access; older supported environments use the ACL API.
    if hasattr(model, 'check_access'):
        model.check_access('read')
    else:
        model.check_access_rights('read')
    def safe(name):
        return not any(s in name.lower() for s in ('password', 'passwd', 'secret', 'token', 'credential', 'api_key', 'apikey', 'private_key', 'privatekey', 'session', 'cookie', 'otp', 'totp', 'signature', 'attachment', 'datas', 'dbfilter', 'config_parameter', 'smtp_pass', 'passphrase', 'access_key', 'encryption'))
    candidates = sorted(n for n, f in model._fields.items() if safe(n) and f.store and f.type in ('boolean', 'integer', 'float', 'monetary', 'char', 'text', 'selection', 'date', 'datetime'))
    available = model.fields_get(candidates, attributes=['string', 'type', 'required', 'readonly', 'store'])
    fields = a.get('fields') or list(available)[:64]
    requested = [n.split(':')[0] for n in fields] + (a.get('group_by') or [])
    requested += [node[0] for node in (a.get('domain') or []) if isinstance(node, list)]
    requested += [part.strip().split()[0] for part in a.get('order', '').split(',') if part.strip()]
    if any(n not in available for n in requested):
        raise ValueError('fields must be accessible, non-sensitive, stored scalar fields')
    limit = a['limit']
    truncated = False
    if p['operation'] == 'model_info':
        result = {n: available[n] for n in fields[:limit]}
        truncated = len(fields) > limit or (not a.get('fields') and len(available) > min(limit, 64))
    elif p['operation'] == 'search_read':
        result = model.search_read(a.get('domain') or [], fields=fields, offset=a.get('offset', 0), limit=limit + 1, order=a.get('order') or 'id')
        truncated = len(result) > limit
        result = result[:limit]
    elif p['operation'] == 'read_group':
        result = model.read_group(a.get('domain') or [], fields, a['group_by'], offset=a.get('offset', 0), limit=limit + 1, orderby=a.get('order') or False, lazy=False)
        truncated = len(result) > limit
        result = result[:limit]
        for row in result:
            row.pop('__domain', None)
            row.pop('__context', None)
            row.pop('__range', None)
    else:
        raise ValueError('unsupported inspection operation')
    print('ODOOCTL_MCP_RESULT=' + json.dumps({'data': result, 'truncated': truncated, 'provenance': {'operation': p['operation'], 'model': a['model'], 'user_id': inspection.uid, 'company_ids': companies, 'read_only': read_only}, 'warning': 'Company IDs select Odoo context, not hard row isolation. Ordinary writes are blocked in this transaction, but registry and custom model code are not sandboxed and can change transaction controls or cause external side effects. Use trusted development code only.'}, default=str))
finally:
    cr.rollback()
`

func databaseParseORMOutput(out []byte) (any, error) {
	if len(out) > databaseORMOutputLimit {
		return nil, fmt.Errorf("ORM inspection output exceeds limit")
	}
	lines := strings.Split(string(out), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSuffix(lines[i], "\r")
		if !strings.HasPrefix(line, databaseORMMarker) {
			continue
		}
		var result map[string]any
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, databaseORMMarker)), &result); err != nil || result == nil {
			return nil, fmt.Errorf("invalid marked ORM inspection output")
		}
		return result, nil
	}
	return nil, fmt.Errorf("ORM inspection result marker missing")
}

func (s *Server) databaseSQL(ctx context.Context, query string, columns []string, table string) (any, error) {
	if err := databaseRole(s.options.SQLRole); err != nil {
		return nil, err
	}
	if err := databaseTables(s.options.SQLTables); err != nil {
		return nil, err
	}
	out, err := s.run(ctx, databaseSQLScript(query, columns, table, s.options.SQLTables), "exec", "-T", "db", "psql", "-X", "--no-password", "--set", "ON_ERROR_STOP=1", "-q", "-t", "-A", "-U", s.options.SQLRole, "-d", s.state.DBName())
	if err != nil {
		return nil, err
	}
	var rows []json.RawMessage
	if err := json.Unmarshal(out, &rows); err != nil {
		return nil, fmt.Errorf("invalid SQL inspection output: %w", err)
	}
	truncated := len(rows) > databaseMaxRows
	if truncated {
		rows = rows[:databaseMaxRows]
	}
	return map[string]any{"data": rows, "truncated": truncated, "provenance": map[string]any{"database": s.state.DBName(), "role": s.options.SQLRole, "table": table, "allowed_tables": s.options.SQLTables, "read_only": true}, "warning": "DBA must restrict this dedicated role's grants, including executable functions and RLS policies; read-only transactions do not prevent external side effects."}, nil
}

func (s *Server) registerDatabase(srv *mcp.Server) {
	addTool(srv, "database_query", "Inspect a restricted SELECT using a DBA-provisioned dedicated read-only role. Requires SQLRole and SQLTables; no arbitrary functions or SQL.", func(ctx context.Context, in databaseQueryInput) (any, error) {
		query, err := databaseValidateSQL(in.Query, s.options.SQLTables)
		if err != nil {
			return nil, err
		}
		return s.databaseSQL(ctx, query.SQL, query.Columns, query.Table)
	})
	addTool(srv, "database_schema", "Inspect at most 200 non-sensitive scalar column definitions from up to 50 explicitly allowed public tables.", func(ctx context.Context, in databaseSchemaInput) (any, error) {
		if err := databaseTables(s.options.SQLTables); err != nil {
			return nil, err
		}
		tables := s.options.SQLTables
		if in.Table != "" {
			if !databaseContains(tables, in.Table) {
				return nil, fmt.Errorf("table is not configured in SQLTables")
			}
			tables = []string{in.Table}
		}
		quoted := make([]string, len(tables))
		for i, table := range tables {
			quoted[i] = databaseSQLString(table)
		}
		// Filter metadata server-side so secret field names never enter tool output.
		query := `SELECT c.table_name, c.column_name, c.data_type, c.is_nullable FROM information_schema.columns c JOIN pg_catalog.pg_class r ON r.relname=c.table_name JOIN pg_catalog.pg_namespace n ON n.oid=r.relnamespace WHERE c.table_schema='public' AND n.nspname='public' AND r.relkind='r' AND c.table_name IN (` + strings.Join(quoted, ",") + `) AND c.data_type IN ('smallint','integer','bigint','real','double precision','numeric','boolean','text','character varying','character','date','timestamp without time zone','timestamp with time zone') AND c.column_name !~* '(password|passwd|secret|token|credential|api_key|apikey|private_key|privatekey|session|cookie|otp|totp|signature|attachment|datas|dbfilter|config_parameter|smtp_pass|passphrase|access_key|encryption)' ORDER BY c.table_name,c.ordinal_position LIMIT 201`
		return s.databaseSQL(ctx, query, nil, "")
	})
	for _, operation := range []string{"model_info", "search_read", "read_group"} {
		addTool(srv, "odoo_"+operation, "Inspect allowed Odoo models as an explicitly configured non-superuser, respecting record rules. Stored non-sensitive scalar fields only; custom code may have external side effects.", func(ctx context.Context, in databaseORMInput) (any, error) {
			script, err := databaseORMScript(in, operation, s.options)
			if err != nil {
				return nil, err
			}
			out, err := s.run(ctx, script, "exec", "-T", "odoo", "odoo", "shell", "-d", s.state.DBName(), "--no-http", "--log-level=critical")
			if err != nil {
				return nil, err
			}
			return databaseParseORMOutput(out)
		})
	}
}
