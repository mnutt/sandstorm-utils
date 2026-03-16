import { execFile } from "node:child_process";
import path from "node:path";
import { promisify } from "node:util";
import { fileURLToPath } from "node:url";

const execFileAsync = promisify(execFile);
const here = path.dirname(fileURLToPath(import.meta.url));
const binDir = process.env.SANDSTORM_UTILS_BIN || path.resolve(here, "..", "dist", "bin");
const sessionId = process.argv[2];
const issueId = process.argv[3] || "42";
const commentId = process.argv[4] || "7";

if (!sessionId) {
  console.error(`usage: node ${path.basename(process.argv[1])} <session-id> [issue-id] [comment-id]`);
  process.exit(1);
}

await execFileAsync(path.join(binDir, "post-activity"), [
  "--path",
  `/issues/${issueId}#comment-${commentId}`,
  "--type",
  "3",
  "--thread-path",
  `/issues/${issueId}`,
  "--thread-title",
  `Issue ${issueId}`,
  "--caption",
  "New comment",
  sessionId,
]);

console.log(`Posted activity for comment ${commentId} on issue ${issueId}.`);
