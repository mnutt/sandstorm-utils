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

const { stdout } = await execFileAsync(path.join(binDir, "get-session-request"), [sessionId]);
const request = JSON.parse(stdout);

console.log(`Session type: ${request.sessionType}`);

for (const [index, descriptor] of request.descriptors.entries()) {
  console.log(`\nDescriptor ${index + 1}`);
  console.log(`  quality: ${descriptor.quality}`);
  console.log(`  tagIds: ${descriptor.tagIds.join(", ")}`);

  for (const tag of descriptor.tags) {
    const summary = tag.knownType || tag.text || "unknown";
    console.log(`  tag ${tag.id}: ${summary}`);
  }
}
