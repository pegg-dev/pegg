#!/usr/bin/env node
'use strict';

const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const https = require('node:https');
const http = require('node:http');
const crypto = require('node:crypto');
const { execFileSync } = require('node:child_process');

const REPO = 'peggco/pegg';
const API_BASE = 'https://api.github.com';
const DL_BASE =
  process.env.PEGG_DOWNLOAD_BASE_URL ||
  `https://github.com/${REPO}/releases/download`;
const LATEST_CKSUM = `https://github.com/${REPO}/releases/latest/download/checksums.txt`;
const UA = 'pegg-installer';
const MAX_REDIRECTS = 5;

function detectPlatform() {
  let plat;
  if (process.platform === 'linux') plat = 'linux';
  else if (process.platform === 'darwin') plat = 'darwin';
  else if (process.platform === 'win32') plat = 'windows';
  else
    throw new Error(
      `unsupported operating system: ${process.platform} (supported: linux, darwin, windows)`
    );

  let arch;
  if (process.arch === 'x64') arch = 'amd64';
  else if (process.arch === 'arm64') arch = 'arm64';
  else
    throw new Error(
      `unsupported architecture: ${process.arch} (supported: amd64, arm64 — use an x64/arm64 Node build)`
    );

  const ext = plat === 'windows' ? 'zip' : 'tar.gz';
  return { plat, arch, ext, assetFile: `pegg_${plat}_${arch}.${ext}` };
}

function existingBinaryWorks(dest) {
  if (!fs.existsSync(dest)) return false;
  try {
    require('node:child_process').execFileSync(dest, ['version'], {
      stdio: 'ignore',
      timeout: 15000,
    });
    return true;
  } catch {
    return false;
  }
}

async function resolveTag() {
  const envv = process.env.PEGG_VERSION;
  if (envv && envv.trim()) {
    const t = envv.trim();
    return t.startsWith('v') ? t : `v${t}`;
  }

  const pkgVersion = require('./package.json').version;
  if (/^\d+\.\d+\.\d+$/.test(pkgVersion)) return `v${pkgVersion}`;

  const headers = {
    'User-Agent': UA,
    Accept: 'application/vnd.github+json',
  };
  if (process.env.GITHUB_TOKEN) {
    headers.Authorization = `Bearer ${process.env.GITHUB_TOKEN}`;
  }
  const res = await fetchJSON(`${API_BASE}/repos/${REPO}/releases/latest`, headers);
  if (!res.json.tag_name) throw new Error('GitHub API response missing tag_name');
  return res.json.tag_name;
}

function fetchJSON(url, headers) {
  return new Promise((resolve, reject) => {
    const get = url.startsWith('http:') ? http.get : https.get;
    const req = get(url, { headers }, (res) => {
      if (res.statusCode === 403 || res.statusCode === 429) {
        res.resume();
        reject(
          new Error(
            'GitHub API rate limited — retry later or set PEGG_VERSION=v0.1.5'
          )
        );
        return;
      }
      if (res.statusCode < 200 || res.statusCode >= 300) {
        res.resume();
        reject(new Error(`GitHub API request failed (HTTP ${res.statusCode})`));
        return;
      }
      let body = '';
      res.setEncoding('utf8');
      res.on('data', (c) => (body += c));
      res.on('end', () => {
        try {
          resolve({ json: JSON.parse(body) });
        } catch (e) {
          reject(new Error(`invalid JSON from GitHub API: ${e.message}`));
        }
      });
    });
    req.setTimeout(30000, () => req.destroy(new Error('GitHub API timed out')));
    req.on('error', reject);
  });
}

function download(url, dest, hops = 0) {
  return new Promise((resolve, reject) => {
    if (hops > MAX_REDIRECTS) {
      reject(new Error(`too many redirects downloading ${url}`));
      return;
    }
    const get = url.startsWith('http:') ? http.get : https.get;
    const req = get(
      url,
      {
        headers: {
          'User-Agent': UA,
          Accept: 'application/octet-stream,*/*',
        },
      },
      (res) => {
        const code = res.statusCode;
        if (code >= 300 && code < 400 && res.headers.location) {
          res.resume();
          const next = new URL(res.headers.location, url).toString();
          download(next, dest, hops + 1).then(resolve, reject);
          return;
        }
        if (code !== 200) {
          res.resume();
          reject(
            new Error(`failed to download ${url} (HTTP ${code}) — release may not exist`)
          );
          return;
        }
        const out = fs.createWriteStream(dest);
        res.pipe(out);
        out.on('finish', () => out.close(resolve));
        out.on('error', reject);
        res.on('error', reject);
      }
    );
    req.setTimeout(30000, () => req.destroy(new Error('download timed out')));
    req.on('error', reject);
  });
}

function verifyChecksum(archivePath, checksumsPath, assetFile) {
  const text = fs.readFileSync(checksumsPath, 'utf8');
  let expected = null;
  for (const line of text.split('\n')) {
    const m = /^([0-9a-fA-F]{64})\s+\*?(.+)$/.exec(line.trim());
    if (!m) continue;
    const name = path.basename(m[2].trim());
    if (name === assetFile) {
      expected = m[1].toLowerCase();
      break;
    }
  }
  if (!expected) throw new Error(`no checksum entry found for ${assetFile}`);

  const actual = crypto
    .createHash('sha256')
    .update(fs.readFileSync(archivePath))
    .digest('hex');
  if (actual !== expected) {
    throw new Error(
      `checksum mismatch for ${assetFile}\n  expected ${expected}\n  actual   ${actual}\n  refusing to install — please retry or report an issue`
    );
  }
}

function extract(archivePath, destDir, assetFile) {
  fs.mkdirSync(destDir, { recursive: true });
  try {
    if (assetFile.endsWith('.tar.gz')) {
      execFileSync('tar', ['-xzf', archivePath, '-C', destDir], {
        stdio: 'ignore',
      });
    } else {
      execFileSync('tar', ['-xf', archivePath, '-C', destDir], {
        stdio: 'ignore',
      });
    }
  } catch (e) {
    if (e.code === 'ENOENT') {
      throw new Error(
        `failed to extract ${assetFile} — system 'tar' is required (macOS/Linux always have it; Windows 10+ ships it)`
      );
    }
    throw new Error(`failed to extract ${assetFile}: ${e.message}`);
  }
}

function locateBinary(dir) {
  const direct = [
    path.join(dir, 'pegg'),
    path.join(dir, 'pegg.exe'),
  ].filter((p) => fs.existsSync(p));
  if (direct.length) return direct[0];

  const stack = [dir];
  while (stack.length) {
    const cur = stack.pop();
    for (const entry of fs.readdirSync(cur, { withFileTypes: true })) {
      const p = path.join(cur, entry.name);
      if (entry.isDirectory()) stack.push(p);
      else if (entry.name === 'pegg' || entry.name === 'pegg.exe') return p;
    }
  }
  return null;
}

function installBinary(src, plat) {
  const destDir = path.join(__dirname, 'bin');
  const dest = path.join(destDir, plat === 'windows' ? 'pegg.exe' : 'pegg');
  fs.mkdirSync(destDir, { recursive: true });
  try {
    fs.renameSync(src, dest);
  } catch (e) {
    if (e.code === 'EXDEV') {
      fs.copyFileSync(src, dest);
      fs.rmSync(src, { force: true });
    } else {
      throw e;
    }
  }
  if (plat !== 'windows') fs.chmodSync(dest, 0o755);
  return dest;
}

async function main() {
  const { plat, ext, assetFile } = detectPlatform();
  const dest = path.join(__dirname, 'bin', plat === 'windows' ? 'pegg.exe' : 'pegg');

  if (existingBinaryWorks(dest)) {
    console.log('pegg: native binary already installed; skipping download');
    return;
  }

  const tag = await resolveTag();
  const tmp = fs.mkdtempSync(path.join(os.tmpdir(), 'pegg-'));
  try {
    const archivePath = path.join(tmp, assetFile);
    await download(`${DL_BASE}/${tag}/${assetFile}`, archivePath);

    const checksumsPath = path.join(tmp, 'checksums.txt');
    try {
      await download(`${DL_BASE}/${tag}/checksums.txt`, checksumsPath);
    } catch {
      await download(LATEST_CKSUM, checksumsPath);
    }

    verifyChecksum(archivePath, checksumsPath, assetFile);
    const extractDir = path.join(tmp, 'extract');
    extract(archivePath, extractDir, assetFile);
    const bin = locateBinary(extractDir);
    if (!bin) throw new Error(`pegg binary not found inside ${assetFile}`);
    fs.chmodSync(bin, 0o755);
    const placed = installBinary(bin, plat);
    console.log(`pegg: installed ${assetFile} (${tag}) → ${placed}`);
  } finally {
    fs.rmSync(tmp, { recursive: true, force: true });
  }
}

main().catch((err) => {
  const msg = err && err.message ? err.message : String(err);
  const code = err && err.code;
  const network =
    code === 'ECONNREFUSED' ||
    code === 'ENOTFOUND' ||
    code === 'EAI_AGAIN' ||
    code === 'ETIMEDOUT';
  console.error(`pegg install failed: ${msg}`);
  if (network) {
    console.error('  no network access to github.com — check your connection/proxy and retry');
  }
  console.error('  retry: npm i -g @peggco/pegg');
  console.error('  pin:   PEGG_VERSION=v0.1.5 npm i -g @peggco/pegg');
  process.exit(1);
});
