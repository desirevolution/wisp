import { createHash } from 'node:crypto';
import { readFileSync, writeFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { defineConfig } from 'vite';
import { fileURLToPath, URL } from 'node:url';

// Tie worker discovery to the actual frontend bytes, including dev-tagged Docker builds.
function versionServiceWorker() {
  return {
    name: 'wisp-version-service-worker',
    writeBundle({ dir }, bundle) {
      const workerPath = resolve(dir, 'sw.js');
      const worker = readFileSync(workerPath, 'utf8');
      const assets = Object.keys(bundle).filter(name => name.startsWith('assets/')).sort();
      const icons = ['icon.svg', 'icon-192.png', 'icon-512.png', 'icon-maskable-512.png'];
      const hash = createHash('sha256').update(worker);
      for (const name of ['index.html', 'manifest.webmanifest', ...icons.map(name => 'icons/' + name), ...assets]) {
        hash.update(name).update(readFileSync(resolve(dir, name)));
      }
      writeFileSync(workerPath, worker
        .replace("'wisp-dev'", `'wisp-build-${hash.digest('hex').slice(0, 16)}'`)
        .replace('/* BUILD_ASSETS */', assets.map(name => `${JSON.stringify('/' + name)},`).join('\n  ')));
    },
  };
}

export default defineConfig({
  plugins: [versionServiceWorker()],
  root: fileURLToPath(new URL('.', import.meta.url)),
  publicDir: 'public',
  build: {
    outDir: '../public',
    emptyOutDir: true,
    sourcemap: false,
  },
  server: {
    proxy: {
      '/api': 'http://127.0.0.1:8080',
    },
  },
});
