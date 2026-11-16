# Notes (fetched 2026-11-16, all verified by reading the page text)

Rules: `app.fluent_bit.tail_inotify_nofile` (runtime).

## Verified
- Tail input page: "To monitor a large number of files, you can increase the inotify settings in your Linux environment by modifying the following sysctl parameters": `fs.inotify.max_user_watches` and `fs.inotify.max_user_instances`, persisted in a sysctl.d file. A systemd override (`/etc/systemd/system/fluent-bit.service.d/override.conf`) with `LimitNOFILE=` "to raise the number of files that the Tail plugin can monitor".
- Same page, inotify queue overflow handling: Fluent Bit reconciles files and retries watch registration that failed "for example due to ENOSPC when fs.inotify.max_user_watches is exhausted".

## Not documented (gaps)
- The docs use placeholders (LIMIT, LIMIT1, LIMIT2) and give no numbers, so no sysctl or limit rule with a recommended value exists. The runtime rule only surfaces the knobs whenever Fluent Bit is detected or declared; it cannot judge whether current values suffice.
- The shipped systemd unit could not be fetched (init/fluent-bit.service.in returned 404), so its defaults are unverified.
- Buffering/troubleshooting pages contain no host-level guidance.
