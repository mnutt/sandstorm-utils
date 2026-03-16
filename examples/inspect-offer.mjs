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

const { stdout } = await execFileAsync(path.join(binDir, "get-session-offer"), [sessionId]);
const offer = JSON.parse(stdout);

console.log(`Session type: ${offer.sessionType}`);
console.log(`Capability present: ${offer.capability.present}`);
console.log(`Descriptor quality: ${offer.descriptor.quality}`);

for (const tag of offer.descriptor.tags) {
  console.log(`- ${tag.id}: ${tag.knownType || tag.text || "unknown"}`);
  if (tag.value) {
    console.log(`  ${JSON.stringify(tag.value)}`);
  }
}
