import net from 'node:net';
import dns from 'node:dns';
import fs from 'node:fs';

// Trusted operator harness, not an MCP evaluation tool. Never emit responses,
// headers, credentials, resolved addresses, or raw exceptions.
const results = [];
const record = (probe, passed, status = 0, byteCount = 0) =>
  results.push({ probe, passed, status, byteCount });
const deadline = setTimeout(() => process.exit(2), 20000);

function socketProbe(host, port, request = '') {
  return new Promise(resolve => {
    let connected = false, byteCount = 0, line = '', done = false;
    const socket = net.connect({ host, port });
    const finish = (status = 0) => {
      if (done) return;
      done = true;
      clearTimeout(timer);
      socket.destroy();
      resolve({ connected, status, byteCount });
    };
    const timer = setTimeout(finish, 2000);
    socket.on('error', () => finish());
    socket.on('end', () => finish());
    socket.on('connect', () => {
      connected = true;
      if (request) socket.write(request);
      else finish();
    });
    socket.on('data', data => {
      byteCount += data.length;
      line += data.toString('ascii', 0, Math.min(data.length, 256));
      const match = /^HTTP\/1\.[01] (\d{3})[^\r\n]*\r\n/.exec(line);
      if (match) finish(Number(match[1]));
      else if (line.length > 256) finish();
    });
  });
}

async function direct(probe, host, port) {
  const result = await socketProbe(host, port);
  record(probe, !result.connected);
}

function lookup(host) {
  return new Promise(resolve => {
    const timer = setTimeout(() => resolve(null), 2000);
    dns.lookup(host, (err, address) => {
      clearTimeout(timer);
      resolve(err ? null : address);
    });
  });
}

async function proxy(probe, target, expected, get = false) {
  const request = get
    ? 'GET http://odoo:8069/web/login HTTP/1.1\r\nHost: odoo:8069\r\nConnection: close\r\n\r\n'
    : `CONNECT ${target} HTTP/1.1\r\nHost: ${target}\r\n\r\n`;
  const result = await socketProbe('proxy', 3128, request);
  record(probe, result.connected && result.status === expected, result.status, result.byteCount);
}

try {
  const resolvConf = fs.readFileSync('/etc/resolv.conf', 'utf8');
  const resolvers = resolvConf
    .split('\n').filter(line => /^nameserver\s/.test(line));
  // Docker custom networks interpose their embedded resolver. Its only
  // upstream must still be the deliberately unusable worker loopback server.
  const embedded = resolvers.length === 1 && /^nameserver\s+127\.0\.0\.11\s*$/.test(resolvers[0]);
  const loopback = resolvers.length === 1 && /^nameserver\s+127\.0\.0\.1\s*$/.test(resolvers[0]);
  record('loopback-only-dns', loopback || (embedded && /^# ExtServers: \[127\.0\.0\.1\]$/m.test(resolvConf)));
  record('docker-embedded-dns', embedded);
  const hosts = fs.readFileSync('/etc/hosts', 'utf8').split('\n');
  record('static-proxy-host', hosts.some(line => {
    const fields = line.trim().split(/\s+/);
    return fields[0] === fixture.proxyIP && fields.slice(1).includes('proxy');
  }));
  await Promise.all([
    direct('direct-odoo', fixture.odooIP, 8069),
    direct('direct-metadata', '169.254.169.254', 80),
    direct('direct-public', '1.1.1.1', 443),
    ...fixture.gateways.map(host => direct('direct-host-gateway', host, 2375)),
    lookup('example.com').then(address => record('unapproved-dns', address === null)),
    lookup('proxy').then(address => record('proxy-dns', address === fixture.proxyIP)),
    proxy('odoo-http', 'odoo:8069', 200, true),
    proxy('odoo-api-connect', 'odoo:8069', 200),
    proxy('approved-cdn', 'cdn.jsdelivr.net:443', 200),
    proxy('unapproved-host', 'example.com:443', 403),
    proxy('approved-private-dns', '127.0.0.1.nip.io:443', 403),
    proxy('private-literal', `${fixture.odooIP}:443`, 403),
    proxy('metadata-literal', '169.254.169.254:443', 403),
    proxy('mapped-ipv6', '[::ffff:127.0.0.1]:443', 403),
    proxy('ula-ipv6', '[fd00::1]:443', 403),
    proxy('linklocal-ipv6', '[fe80::1]:443', 403),
    proxy('non-tls-port', 'cdn.jsdelivr.net:80', 403),
  ]);
  const output = JSON.stringify(results);
  if (Buffer.byteLength(output) > 2048) process.exit(2);
  process.stdout.write(output + '\n', () => process.exit(results.every(r => r.passed) ? 0 : 1));
} catch {
  process.exit(2);
} finally {
  clearTimeout(deadline);
}
