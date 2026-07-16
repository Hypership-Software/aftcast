#!/usr/bin/env node
'use strict';

const { spawnSync } = require('node:child_process');

const PLATFORM_PACKAGES = {
  'darwin arm64': '@aftcast/darwin-arm64',
  'darwin x64': '@aftcast/darwin-x64',
  'linux arm64': '@aftcast/linux-arm64',
  'linux x64': '@aftcast/linux-x64',
  'win32 arm64': '@aftcast/win32-arm64',
  'win32 x64': '@aftcast/win32-x64',
};

const key = `${process.platform} ${process.arch}`;
const pkg = PLATFORM_PACKAGES[key];
if (!pkg) {
  console.error(
    `aftcast: no prebuilt binary for ${key} - use the shell installer or build from source:\n` +
      'https://github.com/Hypership-Software/aftcast#install',
  );
  process.exit(1);
}

const binName = process.platform === 'win32' ? 'aftcast.exe' : 'aftcast';
let binPath;
try {
  binPath = require.resolve(`${pkg}/bin/${binName}`);
} catch {
  console.error(
    `aftcast: the platform package ${pkg} is missing.\n` +
      'npm can skip optional dependencies (--no-optional, some CI setups); reinstall with\n' +
      '  npm install aftcast\n' +
      'or use the shell installer: https://github.com/Hypership-Software/aftcast#install',
  );
  process.exit(1);
}

const result = spawnSync(binPath, process.argv.slice(2), { stdio: 'inherit' });
if (result.error) {
  console.error(`aftcast: failed to launch ${binPath}: ${result.error.message}`);
  process.exit(1);
}
process.exit(result.status === null ? 1 : result.status);
