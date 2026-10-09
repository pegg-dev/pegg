#!/usr/bin/env node
'use strict';

const { spawnSync } = require('node:child_process');
const path = require('node:path');
const fs = require('node:fs');

const binName = process.platform === 'win32' ? 'pegg.exe' : 'pegg';
const binPath = path.join(__dirname, binName);

if (!fs.existsSync(binPath)) {
  console.error(
    'pegg: native binary not found at ' + binPath + '\n' +
      'The binary is downloaded during install. This usually means install\n' +
      'scripts were skipped. Fix by reinstalling without --ignore-scripts:\n' +
      '  npm i -g @peggco/pegg\n' +
      'pnpm users: approve build scripts (pnpm approve-builds) or install with:\n' +
      '  pnpm add -g --config.dangerouslyAllowAllBuilds=true @peggco/pegg\n' +
      'Still stuck? https://github.com/peggco/pegg/issues'
  );
  process.exit(1);
}

const result = spawnSync(binPath, process.argv.slice(2), { stdio: 'inherit' });

if (result.error) {
  console.error('pegg: failed to launch native binary: ' + result.error.message);
  process.exit(1);
}
if (result.signal) {
  const SIGNALS = { SIGHUP: 1, SIGINT: 2, SIGQUIT: 3, SIGKILL: 9, SIGTERM: 15 };
  process.exit(128 + (SIGNALS[result.signal] || 1));
}
process.exit(result.status === null ? 1 : result.status);
