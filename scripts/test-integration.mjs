import { chromium } from 'playwright';
import { spawnSync } from 'node:child_process';
const result = spawnSync('flutter', ['test', '--platform', 'chrome', 'test/emulator_test.dart', ...(process.env.FQL_TEST_NAME ? ['--name',process.env.FQL_TEST_NAME] : [])], {
  cwd: new URL('../tests/dart/', import.meta.url),
  env: { ...process.env, CHROME_EXECUTABLE: process.env.CHROME_EXECUTABLE || chromium.executablePath() },
  stdio: 'inherit',
  timeout: 180000,
});
if (result.error) console.error(result.error.message);
process.exit(result.status ?? 1);
