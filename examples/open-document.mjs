import { execFile } from "node:child_process";
import path from "node:path";
import { promisify } from "node:util";
import { fileURLToPath } from "node:url";

const execFileAsync = promisify(execFile);
const here = path.dirname(fileURLToPath(import.meta.url));
const binDir = process.env.SANDSTORM_UTILS_BIN || path.resolve(here, "..", "dist", "bin");
const sessionId = process.argv[2];
const recordId = process.argv[3] || "doc-123";
const pathInGrain = `/documents/${recordId}`;

if (!sessionId) {
  console.error(`usage: node ${path.basename(process.argv[1])} <session-id> [record-id]`);
  process.exit(1);
}

await execFileAsync(path.join(binDir, "open-view"), ["--path", pathInGrain, "--new-tab", sessionId]);

console.log(`Requested that Sandstorm open ${pathInGrain} in a new tab.`);
