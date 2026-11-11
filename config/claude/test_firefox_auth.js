const assert = require('node:assert/strict');
const fs = require('node:fs');
const vm = require('node:vm');
const source = fs.readFileSync(process.argv[2], 'utf8');

function load(credentials, tls = false, supported = true) {
  const input = typeof credentials === 'string' ? credentials : JSON.stringify(credentials);
  const files = new Set(['/run/guard/proxy-auth.json']);
  const prefs = {};
  const calls = [];
  class Prompter {}
  if (supported) Prompter.prototype.promptAuthInternal = function (...args) {
    calls.push({receiver: this, args});
    return 'original';
  };
  const interfaces = {nsIAuthInformation: {AUTH_PROXY: 2, PREVIOUS_FAILED: 16}, nsIFile: {NORMAL_FILE_TYPE: 0}};
  const classes = {
    '@mozilla.org/file/local;1': {createInstance() { return {
      initWithPath(path) { this.path = path; },
      remove() { files.delete(this.path); },
      create(type, mode) { assert.equal(mode, 0o600); files.add(this.path); },
    }; }},
    '@mozilla.org/network/file-input-stream;1': {createInstance() { return {init() {}}; }},
    '@mozilla.org/scriptableinputstream;1': {createInstance() { return {
      init() {}, available() { return input.length; }, read() { return input; }, close() {},
    }; }},
  };
  const context = {
    Components: {classes, interfaces},
    ChromeUtils: {importESModule() { return {LoginManagerAuthPrompter: Prompter}; }},
    lockPref(name, value) { prefs[name] = value; },
    getenv(name) { return {GUARD_PROXY_HOST: '192.0.2.1', GUARD_PROXY_PORT: '8080', GUARD_PROXY_TLS: tls ? '1' : '0'}[name]; },
  };
  vm.runInNewContext(source, context);
  assert(!files.has('/run/guard/proxy-auth.json'));
  assert(files.has('/run/guard/auth-ready'));
  assert.equal(prefs['network.proxy.type'], 1);
  assert.equal(prefs['network.proxy.no_proxies_on'], '');
  assert.equal(prefs['network.proxy.proxy_over_tls'], tls);
  return {prompter: new Prompter(), calls};
}

(async () => {
  const credentials = {username: 'u@:/"\\', password: 'p\n\u0416\u00e9:$@/"\\'};
  for (const tls of [false, true]) {
    const {prompter, calls} = load(credentials, tls);
    const proxy = {host: '192.0.2.1', port: 8080, type: tls ? 'https' : 'http'};
    const channel = {QueryInterface() { return {proxyInfo: proxy}; }};
    const info = {flags: 2};
    assert.equal(await prompter.promptAuthInternal(channel, 0, info), true);
    assert.equal(info.username, credentials.username);
    assert.equal(info.password, credentials.password);
    assert.equal(calls.length, 0);
    for (const wrong of [null, {...proxy, host: '192.0.2.2'}, {...proxy, port: 8081}, {...proxy, type: 'socks'}]) {
      const rejected = {flags: 2};
      assert.equal(await prompter.promptAuthInternal({QueryInterface() { return {proxyInfo: wrong}; }}, 0, rejected), false);
      assert.equal(rejected.username, undefined);
      assert.equal(rejected.password, undefined);
    }
    const retry = {flags: 2 | 16};
    assert.equal(await prompter.promptAuthInternal(channel, 0, retry), false);
    assert.equal(retry.password, undefined);
    const origin = {flags: 0};
    assert.equal(await prompter.promptAuthInternal(channel, 7, origin), 'original');
    assert.equal(origin.password, undefined);
    assert.equal(calls[0].receiver, prompter);
    assert.deepEqual(calls[0].args, [channel, 7, origin]);
  }
  const empty = load({username: '', password: ''}).prompter;
  assert.equal(await empty.promptAuthInternal({QueryInterface() { return {proxyInfo: {host: '192.0.2.1', port: 8080, type: 'http'}}; }}, 0, {flags: 2}), false);
  for (const invalid of ['{"password":"SECRET_CANARY', 'x'.repeat(65537), {username: 1, password: 'SECRET_CANARY'}]) {
    assert.throws(() => load(invalid), error => error.message === 'Invalid proxy authentication input' && !String(error).includes('SECRET_CANARY'));
  }
  assert.throws(() => load(credentials, false, false), /Unsupported Firefox authentication interface/);
  console.log('PASS proxy auth scope, TLS type, retry denial, origin delegation, malformed input and interface failure');
})().catch(error => { console.error(error); process.exitCode = 1; });
