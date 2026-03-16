import { execFile } from "node:child_process";
import path from "node:path";
import { promisify } from "node:util";
import { fileURLToPath } from "node:url";

const execFileAsync = promisify(execFile);
const here = path.dirname(fileURLToPath(import.meta.url));
const binDir = process.env.SANDSTORM_UTILS_BIN || path.resolve(here, "..", "dist", "bin");
const sessionId = process.argv[2];
const recipient = process.argv[3];

if (!sessionId || !recipient) {
  console.error(`usage: node ${path.basename(process.argv[1])} <session-id> <recipient-email>`);
  process.exit(1);
}

await execFileAsync(path.join(binDir, "send-email"), [
  "--to",
  recipient,
  "--subject",
  "Sandstorm utility test message",
  "--text",
  "This message was sent through sandstorm-utils.",
  sessionId,
]);

console.log(`Sent a test message to ${recipient}.`);
