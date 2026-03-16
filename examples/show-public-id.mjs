import { execFile } from "node:child_process";
import path from "node:path";
import { promisify } from "node:util";
import { fileURLToPath } from "node:url";

const execFileAsync = promisify(execFile);
const here = path.dirname(fileURLToPath(import.meta.url));
const binDir = process.env.SANDSTORM_UTILS_BIN || path.resolve(here, "..", "dist", "bin");
const sessionId = process.argv[2];

if (!sessionId) {
  console.error(`usage: node ${path.basename(process.argv[1])} <session-id>`);
  process.exit(1);
}

const { stdout } = await execFileAsync(path.join(binDir, "get-public-id"), ["--json", sessionId]);
const info = JSON.parse(stdout);

console.log("Public identity");
console.log(`  publicId: ${info.publicId}`);
console.log(`  hostname: ${info.hostname}`);
console.log(`  autoUrl: ${info.autoUrl}`);
console.log(`  demoUser: ${info.isDemoUser}`);

if (info.autoUrl) {
  console.log(`\nSuggested origin for this grain: ${info.autoUrl}`);
}
