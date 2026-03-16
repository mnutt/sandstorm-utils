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

const { stdout } = await execFileAsync(path.join(binDir, "get-user-address"), ["--json", sessionId]);
const user = JSON.parse(stdout);

console.log("Authenticated user");
console.log(`  address: ${user.address}`);
console.log(`  name: ${user.name}`);

console.log("\nExample audit record");
console.log(
  JSON.stringify(
    {
      actor: {
        email: user.address,
        displayName: user.name,
      },
      happenedAt: new Date().toISOString(),
    },
    null,
    2,
  ),
);
