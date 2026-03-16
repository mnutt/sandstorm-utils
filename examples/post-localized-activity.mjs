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

const payload = {
  path: "/issues/42#comment-7",
  type: 3,
  thread: {
    path: "/issues/42",
    title: {
      defaultText: "Issue 42",
      localizations: [
        { locale: "fr", text: "Probleme 42" },
      ],
    },
  },
  notification: {
    caption: {
      defaultText: "New comment",
      localizations: [
        { locale: "fr", text: "Nouveau commentaire" },
      ],
    },
  },
  users: [
    { identityId: "saved-identity-1", mentioned: true },
    { identityId: "saved-identity-2", subscribed: true },
  ],
};

await execFileAsync(path.join(binDir, "post-activity"), ["--json-input", "-", sessionId], {
  input: JSON.stringify(payload),
});

console.log("Posted a localized activity payload with saved-identity recipients.");
