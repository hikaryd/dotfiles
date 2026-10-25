import { execFileSync } from 'node:child_process';
import { fileURLToPath } from 'node:url';
import test from 'node:test';

test('shared proxy keeps Jevgrep working without a Pi wrapper', () => {
  execFileSync('bash', [fileURLToPath(new URL('./test-local-proxy.sh', import.meta.url))], {
    timeout: 30000,
    stdio: 'pipe',
  });
});
