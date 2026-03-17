# Examples

These Node.js scripts are intentionally standalone. Each one shows how an
app might call the compiled utilities as small, typed subprocesses rather
than binding directly to Cap'n Proto.

By default the scripts look for binaries in `../dist/bin`. Override that by
setting `SANDSTORM_UTILS_BIN`.

Example commands:

```bash
node examples/show-public-id.mjs <session-id>
node examples/show-user-address.mjs <session-id>
node examples/open-document.mjs <session-id> [record-id]
node examples/post-comment-activity.mjs <session-id> [issue-id] [comment-id]
node examples/post-localized-activity.mjs <session-id>
node examples/inspect-request.mjs <session-id>
node examples/inspect-offer.mjs <session-id>
node examples/close-after-export.mjs <session-id>
node examples/send-email.mjs <session-id> <recipient-email>
node examples/stay-awake.mjs <session-id>
```

`stay-awake.mjs` demonstrates the intended integration pattern for background
work: spawn `stay-awake` as a child process, keep its stdin open while the job
runs, then close stdin to release the lock and let the helper exit. The lock
also ends if the helper is terminated or, when `--for` is used, when that
duration expires.
