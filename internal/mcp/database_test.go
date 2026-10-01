package mcpsrv

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/mart337i/odooctl/internal/config"
)

func TestDatabaseSQLValidation(t *testing.T) {
	for _, query := range []string{
		"SELECT id, name FROM res_partner",
		"select id from res_partner where name = 'O''Reilly; -- still a string' order by id desc",
		"select id from res_partner where id >= 1 and active = true or name is not null order by name asc, id",
		"select id from res_partner where credit < 1.5",
	} {
		t.Run(query, func(t *testing.T) {
			result, err := databaseValidateSQL(query, []string{"res_partner"})
			if err != nil {
				t.Fatal(err)
			}
			if result.Table != "res_partner" || !strings.Contains(result.SQL, "from ONLY public.res_partner") || !strings.HasSuffix(result.SQL, " LIMIT 201") || len(result.Columns) == 0 {
				t.Fatalf("unexpected validated query: %+v", result)
			}
		})
	}
	for _, query := range []string{
		"", "select * from res_partner", "select id from res_partner;", "select id from res_partner; delete from res_partner",
		"select id from res_partner -- comment", "select id /* comment */ from res_partner", "select id from res_partner /* nested /* comment */ */",
		`select "id" from res_partner`, "select id from public.res_partner", "select id from res_users", "select id from pg_roles",
		"select password from res_partner", "select id from res_partner where api_key = 'x'", "select id from res_partner order by secret",
		"select pg_sleep(1) from res_partner", "select count(id) from res_partner", "select nextval('x') from res_partner",
		"select id::text from res_partner", "select id + 1 from res_partner", "select current_user from res_partner",
		"select id into temp result from res_partner", "with x as (select id from res_partner) select id from x",
		"select id from res_partner union select id from res_users", "select id from res_partner for update",
		"select id from res_partner where id in (select id from res_users)", "select id from res_partner where id = pg_sleep(1)",
		"select id from res_partner where id = id", "select id from res_partner where id = $x$1$x$",
		"select id from res_partner where name = E'\\x'", "select id from res_partner where name = 'a\\b'",
		"select id from res_partner where name = 'unterminated", "select id from res_partner where name = 'line\nbreak'",
		"select id from res_partner where id = 1.2.3", "select id from res_partner where id = -1",
		"select id from res_partner limit 999999", "select id from res_partner offset 5", "select id from res_partner as p",
		"select id from res_partner join res_users on res_users.id = res_partner.id", "select id from res_partner, res_users",
		"select id from res_partner where (id = 1)", "select id from res_partner where id = 1 and",
		"select id from res_partner order by id nulls first", "select id from res_partner where name like 'x'",
		"begin", "commit", "rollback", "copy res_partner to stdout", "explain select id from res_partner",
		"select id from res_partner\n\\! touch /tmp/not-allowed", strings.Repeat("x", 8193),
	} {
		t.Run(query, func(t *testing.T) {
			if result, err := databaseValidateSQL(query, []string{"res_partner"}); err == nil {
				t.Fatalf("unsafe query accepted: %+v", result)
			}
		})
	}
}

func TestDatabaseRoleAndTables(t *testing.T) {
	if err := databaseRole("mcp_reader"); err != nil {
		t.Fatal(err)
	}
	for _, role := range []string{"", "odoo", "postgres", "root", "admin", "administrator", "superuser", "pg_read_all_data", "MCP", "a;select", "-odoo", strings.Repeat("a", 64)} {
		if err := databaseRole(role); err == nil {
			t.Errorf("unsafe role accepted: %q", role)
		}
	}
	for _, tables := range [][]string{nil, {}, {"public.res_partner"}, {"res_partner;"}, {"pg_roles"}, {"ir_config_parameter"}, {"ir_attachment"}, {"res_users_apikeys"}, make([]string, 51)} {
		if err := databaseTables(tables); err == nil {
			t.Errorf("unsafe allowlist accepted: %v", tables)
		}
	}
	if _, err := databaseValidateSQL("select id from res_partner", nil); err == nil {
		t.Fatal("query accepted without allowlist")
	}
}

func TestDatabaseSQLTransactionGuards(t *testing.T) {
	script := databaseSQLScript("select id from ONLY public.res_partner LIMIT 201", []string{"id", "name"}, "res_partner", []string{"res_partner", "sale_order"})
	for _, guard := range []string{
		"BEGIN READ ONLY;", "SET LOCAL statement_timeout = '5s';", "SET LOCAL lock_timeout = '1s';",
		"SET LOCAL idle_in_transaction_session_timeout = '10s';", "SAVEPOINT inspection;", "SET LOCAL search_path = pg_catalog;",
		"SET LOCAL standard_conforming_strings = on;", "SET LOCAL row_security = on;",
		"current_setting('transaction_read_only') <> 'on'", "read-only transaction required",
		"NOT rolsuper", "NOT rolcreaterole", "NOT rolcreatedb", "NOT rolreplication", "NOT rolbypassrls",
		"current_user <> session_user", "c.relkind='r'", "tn.nspname='pg_catalog'", "a.attnum>0", "NOT a.attisdropped", "a.attgenerated=''",
		"r.rolsuper OR r.rolbypassrls OR r.rolname IN ('odoo','postgres')", "pg_catalog.pg_has_role(current_user,r.oid,'USAGE')",
		"c.relname IN ('res_partner','sale_order')", "pg_catalog.pg_has_role(current_user,c.relowner,'USAGE')",
		"RAISE EXCEPTION", "ARRAY['id','name']", "json_agg(row_to_json(result))", "LIMIT 201", "ROLLBACK;",
	} {
		if !strings.Contains(script, guard) {
			t.Errorf("missing guard %q", guard)
		}
	}
	if !strings.HasPrefix(script, "BEGIN READ ONLY;\n") || !strings.HasSuffix(script, "ROLLBACK;\n") || strings.Contains(script, "COMMIT") || strings.Contains(script, "CREATE ROLE") {
		t.Fatal("invalid transaction lifecycle")
	}
	if strings.Index(script, "NOT rolsuper") > strings.Index(script, "json_agg") {
		t.Fatal("role validation occurs after query")
	}
}

func TestDatabaseSQLOwnerGuardsBeforeOutput(t *testing.T) {
	for _, table := range []string{"res_partner", ""} {
		script := databaseSQLScript("select 1 LIMIT 201", []string{"id"}, table, []string{"res_partner", "sale_order"})
		output := strings.Index(script, "SELECT COALESCE(json_agg")
		for _, guard := range []string{"pg_catalog.pg_has_role(current_user,r.oid,'USAGE')", "pg_catalog.pg_has_role(current_user,c.relowner,'USAGE')", "c.relname IN ('res_partner','sale_order')", "SQL role must not have allowed table owner privileges"} {
			if index := strings.Index(script, guard); index < 0 || index >= output {
				t.Errorf("table %q: guard %q missing before output", table, guard)
			}
		}
		if strings.Contains(script, "relforcerowsecurity") || strings.Contains(script, "relrowsecurity") {
			t.Fatal("owner rejection must not depend on RLS flags")
		}
	}
}

func databaseTestOptions() Options {
	return Options{OdooUserID: 7, CompanyIDs: []int{2, 3}, Models: []string{"res.partner", "sale.order"}}
}

func TestDatabaseORMValidation(t *testing.T) {
	options := databaseTestOptions()
	valid := databaseORMInput{Model: "res.partner", Fields: []string{"id", "name"}, Domain: []any{"|", []any{"id", "in", []any{float64(1), float64(2)}}, []any{"name", "=", "O'Reilly"}}, Order: "id desc, name asc", CompanyIDs: []int{2}, Limit: 200, Offset: 10000}
	if err := databaseValidateORM(valid, "search_read", options); err != nil {
		t.Fatal(err)
	}
	if err := databaseValidateORM(databaseORMInput{Model: "sale.order", Fields: []string{"amount_total:sum"}, GroupBy: []string{"state"}}, "read_group", options); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		edit func(*databaseORMInput)
	}{
		{"unknown model", func(a *databaseORMInput) { a.Model = "res.users" }},
		{"model injection", func(a *databaseORMInput) { a.Model = "res.partner']; env.sudo()" }},
		{"field secret", func(a *databaseORMInput) { a.Fields = []string{"password"} }},
		{"dotted field", func(a *databaseORMInput) { a.Fields = []string{"user_id.login"} }},
		{"function field", func(a *databaseORMInput) { a.Fields = []string{"name()"} }},
		{"aggregation on search", func(a *databaseORMInput) { a.Fields = []string{"id:sum"} }},
		{"too many fields", func(a *databaseORMInput) { a.Fields = make([]string, 65) }},
		{"too many groups", func(a *databaseORMInput) { a.GroupBy = make([]string, 5) }},
		{"group on search", func(a *databaseORMInput) { a.GroupBy = []string{"name"} }},
		{"negative limit", func(a *databaseORMInput) { a.Limit = -1 }},
		{"large limit", func(a *databaseORMInput) { a.Limit = 201 }},
		{"negative offset", func(a *databaseORMInput) { a.Offset = -1 }},
		{"large offset", func(a *databaseORMInput) { a.Offset = 10001 }},
		{"unconfigured company", func(a *databaseORMInput) { a.CompanyIDs = []int{9} }},
		{"invalid company", func(a *databaseORMInput) { a.CompanyIDs = []int{0} }},
		{"order secret", func(a *databaseORMInput) { a.Order = "api_key desc" }},
		{"order injection", func(a *databaseORMInput) { a.Order = "id; DROP TABLE x" }},
		{"order function", func(a *databaseORMInput) { a.Order = "random()" }},
		{"order dotted", func(a *databaseORMInput) { a.Order = "user_id.name" }},
		{"too many ordering fields", func(a *databaseORMInput) { a.Order = strings.Repeat("id,", 8) + "id" }},
		{"domain too long", func(a *databaseORMInput) { a.Domain = make([]any, 65) }},
		{"domain secret", func(a *databaseORMInput) { a.Domain = []any{[]any{"session_id", "=", "x"}} }},
		{"domain dotted", func(a *databaseORMInput) { a.Domain = []any{[]any{"user_id.password", "=", "x"}} }},
		{"domain arbitrary object", func(a *databaseORMInput) { a.Domain = []any{map[string]any{"code": "sudo"}} }},
		{"domain incomplete leaf", func(a *databaseORMInput) { a.Domain = []any{[]any{"id", "="}} }},
		{"domain incomplete boolean", func(a *databaseORMInput) { a.Domain = []any{"|", []any{"id", "=", 1}} }},
		{"domain invalid boolean", func(a *databaseORMInput) { a.Domain = []any{"eval"} }},
		{"domain unsafe operator", func(a *databaseORMInput) { a.Domain = []any{[]any{"id", "any", []any{}}} }},
		{"domain nested value", func(a *databaseORMInput) { a.Domain = []any{[]any{"id", "=", map[string]any{"sudo": true}}} }},
		{"domain large string", func(a *databaseORMInput) { a.Domain = []any{[]any{"name", "=", strings.Repeat("a", 1025)}} }},
		{"domain membership scalar", func(a *databaseORMInput) { a.Domain = []any{[]any{"id", "in", 1}} }},
		{"domain scalar list", func(a *databaseORMInput) { a.Domain = []any{[]any{"id", "=", []any{1}}} }},
		{"domain large membership", func(a *databaseORMInput) { a.Domain = []any{[]any{"id", "in", make([]any, 201)}} }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := databaseORMInput{Model: "res.partner"}
			tt.edit(&input)
			if err := databaseValidateORM(input, "search_read", options); err == nil {
				t.Fatal("invalid ORM input accepted")
			}
		})
	}
	for _, uid := range []int{-1, 0, 1} {
		options.OdooUserID = uid
		if err := databaseValidateORM(databaseORMInput{Model: "res.partner"}, "model_info", options); err == nil {
			t.Errorf("unsafe user %d accepted", uid)
		}
	}
	options = databaseTestOptions()
	options.Models = nil
	if err := databaseValidateORM(databaseORMInput{Model: "res.partner"}, "search_read", options); err == nil {
		t.Fatal("missing model allowlist accepted")
	}
}

func TestDatabaseORMGroupValidation(t *testing.T) {
	for _, input := range []databaseORMInput{
		{Model: "sale.order"},
		{Model: "sale.order", Fields: []string{"amount_total:evil"}, GroupBy: []string{"state"}},
		{Model: "sale.order", Fields: []string{"amount_total:sum:evil"}, GroupBy: []string{"state"}},
		{Model: "sale.order", Fields: []string{"amount_total:sum"}, GroupBy: []string{"access_token"}},
		{Model: "sale.order", Fields: []string{"amount_total:sum"}, GroupBy: []string{"date_order:month"}},
	} {
		if err := databaseValidateORM(input, "read_group", databaseTestOptions()); err == nil {
			t.Errorf("unsafe group request accepted: %+v", input)
		}
	}
}

func TestDatabaseORMScriptGuards(t *testing.T) {
	script, err := databaseORMScript(databaseORMInput{Model: "res.partner", Domain: []any{[]any{"name", "=", "O'Reilly\\snowman \u2603"}, []any{"active", "=", true}, []any{"name", "!=", nil}}}, "search_read", databaseTestOptions())
	if err != nil {
		t.Fatal(err)
	}
	for _, guard := range []string{
		"p = json.loads('", "cr.execute(\"SET TRANSACTION READ ONLY\")", "SET LOCAL statement_timeout", "SET LOCAL lock_timeout",
		"cr.execute(\"SHOW transaction_read_only\")", "read_only = cr.fetchone()[0] == 'on'", "if not read_only:", "raise ValueError('read-only transaction required')", "'read_only': read_only",
		"SET LOCAL idle_in_transaction_session_timeout", "SAVEPOINT inspection", "env.clear()", "inspection.su or inspection.uid == 1",
		"api.Environment(cr, p['uid'], {})", "user.company_ids.ids", "c not in configured or c not in allowed", "'allowed_company_ids': companies",
		"f.store", "model.fields_get(candidates", "n not in available", "limit=limit + 1", "result[:limit]", "row.pop('__domain', None)",
		"finally:\n    cr.rollback()", "'truncated': truncated", "'provenance':", "external side effects",
		"print('" + databaseORMMarker + "' + json.dumps(",
	} {
		if !strings.Contains(script, guard) {
			t.Errorf("missing ORM guard %q", guard)
		}
	}
	for _, forbidden := range []string{"cr.execute(\"BEGIN", ".sudo(", "cr.commit(", "eval(", "exec(", "model.write(", "model.create(", "model.unlink("} {
		if strings.Contains(script, forbidden) {
			t.Errorf("unsafe script operation %q", forbidden)
		}
	}
	if _, err := databaseORMScript(databaseORMInput{Model: "res.partner"}, "arbitrary", databaseTestOptions()); err == nil {
		t.Fatal("arbitrary script operation accepted")
	}
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 unavailable; string guards tested, Python parser check skipped")
	}
	// Parse only, without importing Odoo or connecting to a database. Also execute
	// the params preamble alone to verify JSON/Python quoting of booleans and null.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, python, "-c", `import ast,json,sys
s = sys.stdin.read()
ast.parse(s)
ns = {'json': json}
exec(s[s.index('p = json.loads('):s.index('cr = env.cr')], ns)
assert ns['p']['input']['domain'][0][2] == "O'Reilly\\snowman \u2603"
assert ns['p']['input']['domain'][1][2] is True
assert ns['p']['input']['domain'][2][2] is None
assert ns['p']['input']['limit'] == 200
`)
	cmd.Stdin = strings.NewReader(script)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Python quoting or syntax failed: %v\n%s", err, out)
	}
}

func TestDatabaseLiveORMReadOnly(t *testing.T) {
	project, uidString := os.Getenv("ODOOCTL_MCP_TEST_PROJECT"), os.Getenv("ODOOCTL_MCP_TEST_ORM_UID")
	if project == "" || uidString == "" {
		t.Skip("opt-in live test requires ODOOCTL_MCP_TEST_PROJECT and ODOOCTL_MCP_TEST_ORM_UID")
	}
	uid, err := strconv.Atoi(uidString)
	if err != nil || uid <= 1 {
		t.Fatal("ODOOCTL_MCP_TEST_ORM_UID must be a non-superuser ID greater than 1")
	}
	state, err := config.LookupFromDir(project)
	if err != nil {
		t.Fatalf("resolve live test environment: %v", err)
	}
	options := Options{OdooUserID: uid, Models: []string{"ir.module.module"}}
	script, err := databaseORMScript(databaseORMInput{Model: "ir.module.module", Fields: []string{"id", "name"}, Limit: 1}, "search_read", options)
	if err != nil {
		t.Fatal(err)
	}
	// Test-only, fixed zero-row write probe: no rows are changed, and the generated
	// script always rolls back. Require PostgreSQL's read-only error, not ACL denial.
	const savepoint = `    cr.execute("SAVEPOINT inspection")
`
	const probe = `    try:
        cr.execute("UPDATE ir_module_module SET name=name WHERE false")
    except Exception as error:
        if getattr(error, 'pgcode', None) != '25006':
            raise
        cr.execute("ROLLBACK TO SAVEPOINT inspection")
    else:
        raise AssertionError('zero-row UPDATE was not blocked by read-only transaction')
`
	if strings.Count(script, savepoint) != 1 {
		t.Fatal("generated inspection savepoint missing or ambiguous")
	}
	script = strings.Replace(script, savepoint, savepoint+probe, 1)
	s := &Server{state: state, options: options}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	out, err := s.run(ctx, script, "exec", "-T", "odoo", "odoo", "shell", "-d", state.DBName(), "--no-http", "--log-level=critical")
	if err != nil {
		t.Fatalf("live read-only transaction probe: %v", err)
	}
	result, err := databaseParseORMOutput(out)
	if err != nil {
		t.Fatal(err)
	}
	obj := result.(map[string]any)
	provenance, ok := obj["provenance"].(map[string]any)
	if !ok || provenance["read_only"] != true || provenance["user_id"] != float64(uid) {
		t.Fatalf("live transaction not verified read-only for configured user: %+v", provenance)
	}
	rows, ok := obj["data"].([]any)
	if !ok || len(rows) > 1 {
		t.Fatal("fixed ORM query did not return a bounded result")
	}
}

func TestDatabasePythonRuntimeGuards(t *testing.T) {
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 unavailable; mocked ORM runtime checks skipped")
	}
	// Model psycopg's implicit BEGIN before the first execute after rollback.
	// An explicit nested BEGIN leaves the transaction writable, as on PostgreSQL.
	// Also check field filtering, rollback, bounds, and company/user validation.
	harness := `import json,sys,types,contextlib,io
script = sys.stdin.read()
class Cursor:
    def __init__(self, report='on'):
        self.events=[]; self.active=True; self.read_only=False; self.report=report
    def rollback(self):
        self.events.append('rollback'); self.active=False; self.read_only=False
    def execute(self, sql):
        if not self.active:
            self.events.append('implicit BEGIN'); self.active=True
        self.events.append(sql)
        if sql == 'SET TRANSACTION READ ONLY': self.read_only=True
    def fetchone(self):
        assert self.events[-1] == 'SHOW transaction_read_only'
        self.events.append('fetchone')
        return ['on' if self.read_only and self.report == 'on' else 'off']
class Field:
    def __init__(self, store=True, kind='char'): self.store=store; self.type=kind
class Model:
    _fields={'id':Field(kind='integer'),'name':Field(),'password':Field(),'computed':Field(False),'blob':Field(kind='binary'),'relation':Field(kind='many2one')}
    def check_access(self, op): assert op == 'read'
    def fields_get(self, fields, attributes): return {n:{'type':self._fields[n].type} for n in fields}
    def search_read(self, domain, fields, offset, limit, order):
        assert fields == ['id', 'name'] and limit == 201 and order == 'id'
        if fail: raise ValueError('custom read failed')
        return [{'id':i} for i in range(limit)]
    def read_group(self, domain, fields, groupby, offset, limit, orderby, lazy):
        assert fields == ['id:count'] and groupby == ['name'] and limit == 201 and lazy is False
        return [{'id':i, '__domain':[['password','=','not exposed']], '__context':{'unsafe':True}, '__range':{}} for i in range(limit)]
class Environment:
    def __init__(self, cr, uid, context):
        assert cr.events[:5] == ['rollback','implicit BEGIN','SET TRANSACTION READ ONLY','SHOW transaction_read_only','fetchone']
        assert cr.read_only and cr.report == 'on' and 'SAVEPOINT inspection' in cr.events
        cr.events.append('inspection environment')
        self.cr=cr; self.uid=uid; self.su=superuser
        self.user=types.SimpleNamespace(active=True, exists=lambda:True, company_ids=types.SimpleNamespace(ids=[2,3]), company_id=types.SimpleNamespace(id=2))
    def __getitem__(self, name): return Model()
    def clear(self): pass
api=types.SimpleNamespace(Environment=Environment)
sys.modules['odoo']=types.SimpleNamespace(api=api)
for fail,superuser,companies,fields,report,expected_error in [(False,False,[2],[],'on',False),(True,False,[2],[],'on',True),(False,True,[2],[],'on',True),(False,False,[9],[],'on',True),(False,False,[2],['computed'],'on',True),(False,False,[2],['relation'],'on',True),(False,False,[2],['blob'],'on',True),(False,False,[2],[],'off',True)]:
    cr=Cursor(report)
    env=types.SimpleNamespace(cr=cr,clear=lambda:cr.events.append('env.clear'))
    preamble=script[:script.index('cr = env.cr')]
    body=script[script.index('cr = env.cr'):]
    ns={'env':env}
    exec(preamble,ns)
    ns['p']['input']['company_ids']=companies
    ns['p']['input']['fields']=fields
    output=io.StringIO()
    caught=False
    with contextlib.redirect_stdout(output):
        try: exec(body,ns)
        except ValueError: caught=True
    assert caught == expected_error
    assert cr.events[-1] == 'rollback' and cr.events.count('rollback') == 2
    assert not any('COMMIT' in e for e in cr.events)
    assert 'BEGIN READ ONLY' not in cr.events
    if report == 'off':
        assert cr.events == ['rollback','implicit BEGIN','SET TRANSACTION READ ONLY','SHOW transaction_read_only','fetchone','rollback']
        assert output.getvalue() == ''
    if not expected_error:
        assert output.getvalue().startswith('ODOOCTL_MCP_RESULT=')
        result=json.loads(output.getvalue().split('=',1)[1])
        assert len(result['data']) == 200 and result['truncated'] is True
        assert result['provenance']['user_id'] == 7 and result['provenance']['company_ids'] == [2]
        assert result['provenance']['read_only'] is True
for operation in ['model_info','read_group']:
    fail=superuser=False
    cr=Cursor()
    ns={'env':types.SimpleNamespace(cr=cr,clear=lambda:None)}
    exec(preamble,ns)
    ns['p']['operation']=operation
    if operation == 'read_group':
        ns['p']['input']['fields']=['id:count']
        ns['p']['input']['group_by']=['name']
    output=io.StringIO()
    with contextlib.redirect_stdout(output): exec(body,ns)
    assert output.getvalue().startswith('ODOOCTL_MCP_RESULT=')
    result=json.loads(output.getvalue().split('=',1)[1])
    assert cr.events[-1] == 'rollback'
    assert result['provenance']['read_only'] is True
    if operation == 'model_info':
        assert set(result['data']) == {'id','name'} and result['truncated'] is False
    else:
        assert len(result['data']) == 200 and result['truncated'] is True
        assert all(set(row) == {'id'} for row in result['data'])
`
	script, err := databaseORMScript(databaseORMInput{Model: "res.partner"}, "search_read", databaseTestOptions())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, python, "-c", harness)
	cmd.Stdin = strings.NewReader(script)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("mock ORM checks failed: %v\n%s", err, out)
	}
}

func TestDatabaseParseORMOutput(t *testing.T) {
	for _, output := range []string{
		databaseORMMarker + `{"data":[],"truncated":false}`,
		"Odoo shell banner\n{\"untrusted\":true}\n" + databaseORMMarker + `{"data":[],"truncated":false}` + "\nshutdown message\n",
		databaseORMMarker + "{bad earlier marker}\n" + databaseORMMarker + `{"data":[],"truncated":false}` + "\r\n",
		databaseORMMarker + `{"data":["earlier"]}` + "\n" + databaseORMMarker + `{"data":[],"truncated":false}`,
	} {
		result, err := databaseParseORMOutput([]byte(output))
		if err != nil {
			t.Fatal(err)
		}
		obj := result.(map[string]any)
		if data, ok := obj["data"].([]any); !ok || len(data) != 0 || obj["truncated"] != false {
			t.Fatalf("did not select final marked result: %+v", obj)
		}
	}
	for _, output := range []string{
		"", `{"data":[]}`, "banner " + databaseORMMarker + `{"data":[]}`,
		databaseORMMarker, databaseORMMarker + "null", databaseORMMarker + "[]",
		databaseORMMarker + `{"data":[]} trailing noise`, databaseORMMarker + "{\n\"data\":[]\n}",
		databaseORMMarker + `{"data":[]}` + "\n" + databaseORMMarker + "{bad final marker}",
		strings.Repeat("x", databaseORMOutputLimit+1),
	} {
		if _, err := databaseParseORMOutput([]byte(output)); err == nil {
			t.Errorf("accepted invalid marked output (length %d)", len(output))
		}
	}
}

func TestDatabaseSensitiveNames(t *testing.T) {
	for _, name := range []string{"password", "SMTP_PASS", "api_key", "access_token", "private_key", "totp_secret", "session_id", "ir.config_parameter", "ir_attachment", "datas"} {
		if !databaseSensitive(name) {
			t.Errorf("sensitive name accepted: %s", name)
		}
	}
	if databaseSensitive("name") || databaseSensitive("amount_total") {
		t.Fatal("ordinary fields excluded")
	}
	// Guard against Go/Python/schema exclusion lists drifting apart.
	for _, part := range []string{"password", "smtp_pass", "config_parameter", "access_key", "encryption", "datas"} {
		if !strings.Contains(databasePython, "'"+part+"'") {
			t.Errorf("Python missing sensitive exclusion %q", part)
		}
	}
}

func TestDatabaseORMJSONInput(t *testing.T) {
	var input databaseORMInput
	if err := json.Unmarshal([]byte(`{"model":"res.partner","domain":[["id","in",[1,2]],["active","=",true]],"fields":["id"],"company_ids":[2]}`), &input); err != nil {
		t.Fatal(err)
	}
	if err := databaseValidateORM(input, "search_read", databaseTestOptions()); err != nil {
		t.Fatal(err)
	}
}
