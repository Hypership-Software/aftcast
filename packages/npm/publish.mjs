#!/usr/bin/env node
// Stages and publishes the aftcast npm packages from prebuilt release binaries:
// six platform packages (bare binary + os/cpu constraints) and the `aftcast`
// meta-package whose optionalDependencies pin them at the same version.
//
//   node publish.mjs <version> <distDir>                 publish to the registry
//   node publish.mjs <version> <distDir> --pack <outDir> npm-pack tarballs locally
//
// distDir holds the release build tree: <goos>-<goarch>/aftcast[.exe].
// Publishing requires all six binaries; --pack accepts a subset for local tests.
// Already-published versions are skipped so a release re-run is idempotent.
import { spawnSync } from 'node:child_process';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const TARGETS = [
  { goos: 'darwin', goarch: 'amd64', os: 'darwin', cpu: 'x64' },
  { goos: 'darwin', goarch: 'arm64', os: 'darwin', cpu: 'arm64' },
  { goos: 'linux', goarch: 'amd64', os: 'linux', cpu: 'x64' },
  { goos: 'linux', goarch: 'arm64', os: 'linux', cpu: 'arm64' },
  { goos: 'windows', goarch: 'amd64', os: 'win32', cpu: 'x64' },
  { goos: 'windows', goarch: 'arm64', os: 'win32', cpu: 'arm64' },
];

function fail(msg) {
  console.error(`publish: ${msg}`);
  process.exit(1);
}

const args = process.argv.slice(2);
let packDir = null;
const packIdx = args.indexOf('--pack');
if (packIdx >= 0) {
  if (!args[packIdx + 1]) fail('--pack needs a directory');
  packDir = path.resolve(args[packIdx + 1]);
  args.splice(packIdx, 2);
  fs.mkdirSync(packDir, { recursive: true });
}
const [version, distArg] = args;
if (!version || !distArg) fail('usage: node publish.mjs <version> <distDir> [--pack <outDir>]');
if (!/^\d+\.\d+\.\d+(-[0-9A-Za-z.-]+)?$/.test(version)) {
  fail(`version '${version}' must be bare semver (no leading v)`);
}
const dist = path.resolve(distArg);
const metaSrc = path.join(path.dirname(fileURLToPath(import.meta.url)), 'aftcast');

function npm(cwd, ...npmArgs) {
  const r = spawnSync('npm', npmArgs, {
    cwd,
    stdio: 'inherit',
    shell: process.platform === 'win32',
  });
  if (r.status !== 0) fail(`npm ${npmArgs.join(' ')} failed for ${cwd}`);
}

async function alreadyPublished(name) {
  const res = await fetch(`https://registry.npmjs.org/${encodeURIComponent(name)}/${version}`);
  return res.status === 200;
}

// Platform packages are scoped: unscoped <name>-win32-x64 style names from a
// young account trip npm's spam detection (observed live on the first v0.1.0
// publish attempt); a scope is an owned namespace, so the heuristics don't apply.
function stagePlatform(target) {
  const name = `@aftcast/${target.os}-${target.cpu}`;
  const binName = target.goos === 'windows' ? 'aftcast.exe' : 'aftcast';
  const srcBin = path.join(dist, `${target.goos}-${target.goarch}`, binName);
  if (!fs.existsSync(srcBin)) return null;
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), `aftcast-${target.os}-${target.cpu}-`));
  fs.mkdirSync(path.join(dir, 'bin'));
  fs.copyFileSync(srcBin, path.join(dir, 'bin', binName));
  fs.chmodSync(path.join(dir, 'bin', binName), 0o755);
  const pkg = {
    name,
    version,
    description: `Aftcast prebuilt binary for ${target.os}/${target.cpu}. Install the 'aftcast' package instead of this one.`,
    os: [target.os],
    cpu: [target.cpu],
    files: ['bin'],
    license: 'Apache-2.0',
    repository: { type: 'git', url: 'git+https://github.com/Hypership-Software/aftcast.git' },
  };
  fs.writeFileSync(path.join(dir, 'package.json'), JSON.stringify(pkg, null, 2) + '\n');
  return { name, dir };
}

function stageMeta(platforms) {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'aftcast-meta-'));
  fs.mkdirSync(path.join(dir, 'bin'));
  fs.copyFileSync(path.join(metaSrc, 'bin', 'aftcast.js'), path.join(dir, 'bin', 'aftcast.js'));
  fs.copyFileSync(path.join(metaSrc, 'README.md'), path.join(dir, 'README.md'));
  const pkg = JSON.parse(fs.readFileSync(path.join(metaSrc, 'package.json'), 'utf8'));
  pkg.version = version;
  pkg.optionalDependencies = Object.fromEntries(platforms.map((p) => [p.name, version]));
  fs.writeFileSync(path.join(dir, 'package.json'), JSON.stringify(pkg, null, 2) + '\n');
  return { name: 'aftcast', dir };
}

if (!packDir && process.platform === 'win32') {
  fail('publishing from Windows would drop the exec bit on the unix binaries - publish from the release workflow (or any unix machine); --pack still works here');
}
const platforms = TARGETS.map(stagePlatform).filter(Boolean);
if (platforms.length === 0) fail(`no platform binaries found under ${dist}`);
if (!packDir && platforms.length !== TARGETS.length) {
  fail(
    `publishing requires all ${TARGETS.length} platform binaries under ${dist}, found ${platforms.length} (--pack accepts a subset)`,
  );
}

// The meta-package goes last so it never points at an unpublished platform.
for (const { name, dir } of [...platforms, stageMeta(platforms)]) {
  if (packDir) {
    npm(dir, 'pack', '--pack-destination', packDir);
    console.log(`packed ${name}@${version}`);
  } else if (await alreadyPublished(name)) {
    console.log(`skipped ${name}@${version} (already on the registry)`);
  } else {
    npm(dir, 'publish', '--access', 'public');
    console.log(`published ${name}@${version}`);
  }
}
