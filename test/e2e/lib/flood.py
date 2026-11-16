# Listener that never accept()s plus a connection flood: fills the accept
# queue so the kernel counts TcpExt.ListenOverflows. Usage: flood.py SECONDS
import socket, sys, time, errno
dur = float(sys.argv[1])
ls = socket.socket()
ls.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
ls.bind(("127.0.0.1", 9999))
ls.listen(1)  # tiny backlog; never accepted
end = time.time() + dur
held = []
while time.time() < end:
    for _ in range(50):
        c = socket.socket()
        c.setblocking(False)
        try:
            c.connect_ex(("127.0.0.1", 9999))
        except OSError:
            pass
        held.append(c)
    if len(held) > 300:
        for c in held[:200]:
            c.close()
        held = held[200:]
    time.sleep(0.05)
