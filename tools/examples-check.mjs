#!/usr/bin/env node
import { readFileSync, statSync } from 'node:fs';
import { spawnSync } from 'node:child_process';

const requiredFiles = [
  'examples/README.md',
  'examples/curl/gateway.sh',
  'examples/typescript/gateway.ts',
  'examples/python/gateway.py',
  'docs/MIGRATION_GUIDE_2API.md',
  'tools/examples-check.mjs',
];

const requiredRoutes = [
  '/v1/models',
  '/v1/chat/completions',
  '/v1/responses',
  '/v1/messages',
  '/v1beta/models',
  '/v1beta/models/{model}:countTokens',
  '/v1/messages/count_tokens',
  '/api/v1/admin/ops/realtime/slots',
];

const routeAliases = new Map([
  ['/v1beta/models/{model}:countTokens', ['/v1beta/models/{model}:countTokens', ':countTokens']],
]);

const requiredEnvVars = [
  'SRAPI_BASE_URL',
  'SRAPI_API_KEY',
  'SRAPI_MODEL',
  'SRAPI_GEMINI_MODEL',
  'SRAPI_ADMIN_SESSION',
  'SRAPI_CSRF_TOKEN',
];

const migrationRequiredPhrases = [
  'selected Provider Account',
  'OAuth/session/desktop/CLI/IDE credential',
  '不是把本地 Codex / Claude Code / Antigravity 客户端作为 SRapi 的下游入口',
  '不是在 Gateway service 为 Codex / Claude Code / Antigravity 增加本地 DTO',
  '/home/senran/Desktop/sub2api',
  '/home/senran/Desktop/CLIProxyAPI',
  '/home/senran/Desktop/chatgpt2api',
];

const docs = read('examples/README.md')
  + '\n' + read('examples/curl/gateway.sh')
  + '\n' + read('examples/typescript/gateway.ts')
  + '\n' + read('examples/python/gateway.py')
  + '\n' + read('docs/MIGRATION_GUIDE_2API.md');

function main() {
  for (const file of requiredFiles) {
    assert(statSync(file).isFile(), `${file} is missing`);
  }
  assert((statSync('examples/curl/gateway.sh').mode & 0o111) !== 0, 'examples/curl/gateway.sh must be executable');

  for (const route of requiredRoutes) {
    const aliases = routeAliases.get(route) ?? [route];
    assert(aliases.some((alias) => docs.includes(alias)), `examples/docs missing route ${route}`);
  }

  for (const name of requiredEnvVars) {
    assert(docs.includes(name), `examples/docs missing env var ${name}`);
  }

  const migrationGuide = read('docs/MIGRATION_GUIDE_2API.md');
  for (const phrase of migrationRequiredPhrases) {
    assert(migrationGuide.includes(phrase), `migration guide missing required 2api boundary phrase: ${phrase}`);
  }

  assertNoSecretPlaceholders(docs);
  assert(read('README.md').includes('examples/README.md'), 'README.md must link examples/README.md');
  assert(read('README.md').includes('docs/MIGRATION_GUIDE_2API.md'), 'README.md must link docs/MIGRATION_GUIDE_2API.md');
  assert(read('docs/README.md').includes('MIGRATION_GUIDE_2API.md'), 'docs/README.md must link migration guide');
  assert(read('specs/QUALITY_GATES.md').includes('make examples-check'), 'QUALITY_GATES.md must document make examples-check');

  runTypeScriptCheck();
  console.log('examples check ok');
}

function read(path) {
  return readFileSync(path, 'utf8');
}

function assert(condition, message) {
  if (!condition) {
    throw new Error(message);
  }
}

function assertNoSecretPlaceholders(content) {
  const forbiddenPatterns = [
    /sk-[A-Za-z0-9]{16,}/,
    /Bearer\s+(?!\$\{?SRAPI_API_KEY\}?)[A-Za-z0-9._-]{20,}/,
    /(access_token|refresh_token|session_token)"?\s*[:=]\s*"?[A-Za-z0-9._-]{16,}/i,
    /srapi_session=[A-Za-z0-9._-]{16,}/,
  ];
  for (const pattern of forbiddenPatterns) {
    assert(!pattern.test(content), `examples/docs include a forbidden secret-like placeholder: ${pattern}`);
  }
}

function runTypeScriptCheck() {
  const result = spawnSync(
    'npx',
    [
      '--yes',
      '-p',
      'typescript@5.9.3',
      'tsc',
      '--noEmit',
      '--strict',
      '--module',
      'ESNext',
      '--moduleResolution',
      'Bundler',
      '--target',
      'ES2022',
      '--lib',
      'ES2022,DOM,DOM.Iterable',
      'examples/typescript/gateway.ts',
    ],
    { encoding: 'utf8' },
  );
  if (result.status !== 0) {
    throw new Error(`TypeScript example does not typecheck:\n${result.stdout}${result.stderr}`);
  }
}

try {
  main();
} catch (error) {
  console.error(error instanceof Error ? error.message : String(error));
  process.exit(1);
}
