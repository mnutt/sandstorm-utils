import { spawn } from "node:child_process";
import path from "node:path";
import { fileURLToPath } from "node:url";

const here = path.dirname(fileURLToPath(import.meta.url));
const binDir = process.env.SANDSTORM_UTILS_BIN || path.resolve(here, "..", "dist", "bin");
const sessionId = process.argv[2];

if (!sessionId) {
  console.error(`usage: node ${path.basename(process.argv[1])} <session-id>`);
  process.exit(1);
}

const stayAwake = path.join(binDir, "stay-awake");

function sleep(ms) {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

// Keep this child process alive while background work is running.
// The wake lock is released when the helper exits, its stdin closes,
// or it receives SIGTERM/SIGHUP/SIGINT.
const child = spawn(
  stayAwake,
  [
    "--title",
    "Transcoding video",
    "--caption",
    "Encoding a video in the background",
    sessionId,
  ],
  {
    stdio: ["pipe", "inherit", "inherit"],
  }
);

try {
  console.log("Wake-lock helper started. The lock remains active while this child is running.");
  await sleep(30_000);
  await sleep(30_000);
  console.log("Background work complete.");
} finally {
  // Closing stdin tells the helper to release the lock and exit cleanly.
  if (child.stdin) {
    child.stdin.end();
  }

  await new Promise((resolve, reject) => {
    child.once("error", reject);
    child.once("exit", (code) => {
      if (code === 0) {
        resolve();
        return;
      }
      reject(new Error(`stay-awake exited with code ${code}`));
    });
  });
}
