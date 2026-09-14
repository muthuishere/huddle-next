#!/usr/bin/env node
'use strict';

const { spawn } = require('child_process');

const SUPPORTED = {
  'darwin-arm64': '@muthuishere/huddle-darwin-arm64',
  'darwin-x64': '@muthuishere/huddle-darwin-x64',
  'linux-arm64': '@muthuishere/huddle-linux-arm64',
  'linux-x64': '@muthuishere/huddle-linux-x64',
  'win32-x64': '@muthuishere/huddle-windows-x64',
};

const key = `${process.platform}-${process.arch}`;
const pkg = SUPPORTED[key];

function fail(message) {
  process.stderr.write(`huddle: ${message}\n`);
  process.exit(1);
}

if (!pkg) {
  fail(`unsupported platform ${key}. Supported: ${Object.keys(SUPPORTED).join(', ')}`);
}

const binName = process.platform === 'win32' ? 'huddle.exe' : 'huddle';

let binPath;
try {
  binPath = require.resolve(`${pkg}/bin/${binName}`);
} catch (_err) {
  fail(
    `platform package ${pkg} is not installed. Reinstall with: ` +
      `npm install -g @muthuishere/huddle`,
  );
}

const child = spawn(binPath, process.argv.slice(2), { stdio: 'inherit' });
child.on('exit', (code, signal) => {
  if (signal) {
    process.kill(process.pid, signal);
    return;
  }
  process.exit(code ?? 1);
});
child.on('error', (err) => fail(`failed to launch ${binPath}: ${err.message}`));
