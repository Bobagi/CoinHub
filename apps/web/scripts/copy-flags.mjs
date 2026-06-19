// Copies the flag-icons 4x3 SVGs into dist/flags so the app can serve country flags from its own
// origin (/flags/<iso2>.svg). Same-origin is required by our CSP (img-src 'self'), which blocks
// external flag CDNs — and local SVGs render identically on Windows, where emoji flags do not.
import { cpSync, existsSync, mkdirSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

const scriptDirectory = dirname(fileURLToPath(import.meta.url))
const sourceDirectory = resolve(scriptDirectory, '../node_modules/flag-icons/flags/4x3')
const destinationDirectory = resolve(scriptDirectory, '../dist/flags')

if (!existsSync(sourceDirectory)) {
  console.error(`copy-flags: flag-icons not found at ${sourceDirectory} (run pnpm install)`)
  process.exit(1)
}
mkdirSync(destinationDirectory, { recursive: true })
cpSync(sourceDirectory, destinationDirectory, { recursive: true })
console.log(`copy-flags: copied country flags → ${destinationDirectory}`)
