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

const [{ stdout: userJSON }, { stdout: publicIdJSON }] = await Promise.all([
  execFileAsync(path.join(binDir, "get-user-address"), ["--json", sessionId]),
  execFileAsync(path.join(binDir, "get-public-id"), ["--json", sessionId]),
]);
const user = JSON.parse(userJSON);
const publicId = JSON.parse(publicIdJSON);

console.log("Preparing export metadata before closing the session...");
console.log(
  JSON.stringify(
    {
      actor: user,
      grain: {
        publicId: publicId.publicId,
        autoUrl: publicId.autoUrl,
      },
    },
    null,
    2,
  ),
);

await execFileAsync(path.join(binDir, "close-session"), [sessionId]);
console.log("Session close requested.");
