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

const stayAwake = path.join(binDir, "stay-awake");
const ttl = "2m";

function sleep(ms) {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

let lockId;

try {
  const { stdout: acquireJSON } = await execFileAsync(stayAwake, [
    "acquire",
    "--ttl",
    ttl,
    "--title",
    "Transcoding video",
    "--caption",
    "Encoding a video in the background",
    sessionId,
  ]);
  const acquired = JSON.parse(acquireJSON);
  lockId = acquired.lockId;

  console.log(`Acquired wake lock ${lockId} until ${acquired.expiresAt}.`);
  console.log("Starting background work...");

  await sleep(30_000);

  const { stdout: renewJSON } = await execFileAsync(stayAwake, [
    "renew",
    "--ttl",
    ttl,
    lockId,
  ]);
  const renewed = JSON.parse(renewJSON);

  console.log(`Renewed wake lock until ${renewed.expiresAt}.`);

  await sleep(30_000);
  console.log("Background work complete.");
} finally {
  if (lockId) {
    await execFileAsync(stayAwake, ["release", lockId]);
    console.log(`Released wake lock ${lockId}.`);
  }
}
