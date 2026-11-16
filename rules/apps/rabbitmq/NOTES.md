# RabbitMQ notes (fetched 2026-11-16, verified by reading the page text)

www.rabbitmq.com sits behind a Cloudflare challenge and cannot be fetched with curl. The rules cite the official docs source in rabbitmq/rabbitmq-website (`docs/production-checklist.md`, `docs/networking.md`), which renders to the www.rabbitmq.com/docs pages of the same name.

## Verified
- Production checklist: "Make sure production environments allow for at least 50K open file descriptors for effective RabbitMQ user"; "multiply the 95th percentile number of concurrent connections by 2 and add the total number of queues"; values as high as 500K "are recommended for production setups". -> `app.rabbitmq.nofile` (50000).
- Networking guide, kernel table: `net.core.somaxconn` "Default is 128. Increase to 4096 or higher to support inbound connection bursts"; `net.ipv4.tcp_max_syn_backlog` "4096 and 8192 are recommended starting values when optimising for throughput"; `net.ipv4.tcp_fin_timeout` "Lowering this timeout to a value in the 15-30 second range"; example keepalive config `tcp_keepalive_time=30`, `intvl=10`, `probes=4`. -> the matching `app.rabbitmq.*` sysctl rules.
- Open file handle section: "When overriding the max open file handle limit to a higher value, the ERL_MAX_PORTS environment variable must be overridden accordingly"; default "usually set to 65536". -> `app.rabbitmq.erl_max_ports` (runtime).
- Connection backlog: `tcp_listen_options.backlog` default 128; "connections will be rejected by the operating system. See also net.core.somaxconn". -> `app.rabbitmq.listen_backlog` (runtime, gated on ListenOverflows).

## The docs do NOT say
- Numeric values for `fs.file-max`, `net.ipv4.ip_local_port_range` (only "enough entries for the peak number of concurrent connections"), or TCP buffer sizes (`tcp_listen_options.sndbuf/recbuf` are app settings; kernel buffer tuning is pointed at an external fasterdata.es.net page and stated as "not necessarily" better).
- A vm.swappiness, THP, scheduler or filesystem recommendation. The memory pages are about the Erlang VM high watermark; the checklist only warns that too little free memory causes "OS swapping".
- `net.ipv4.tcp_tw_reuse = 1` is documented but only for client hosts or intermediaries without NAT, so it is not a rule for the broker host.
- The "50K" figure is not given as exactly 50000 or 51200; the rule uses 50000.

## Engine gaps seen
- The keepalive example is a three-sysctl set, expressed as three independent rules.
- Detection relies on `beam.smp` with `-s rabbit` or "rabbit" in the cmdline; other Erlang apps are not matched.
