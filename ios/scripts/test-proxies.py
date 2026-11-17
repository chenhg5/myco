#!/usr/bin/env python3
"""Four small forwarding proxies for the app's proxy settings UI test
(WalkthroughTests.testAProxySetHereIsTriedAndSaved): HTTP (CONNECT), HTTP
with the password me:pw, SOCKS5, SOCKS5 with me:pw, on four consecutive
ports of this machine. Each relays the tunnel to where it was asked to go
and logs what it saw, never what went through.

    python3 ios/scripts/test-proxies.py /tmp/proxies.log 18470 &
    TEST_RUNNER_MYCO_PROXY_PORT=18470 \
    TEST_RUNNER_MYCO_PROXY_TARGET=https://<an address the proxies reach>/ \
      xcodebuild test ... -only-testing:MycoAppUITests/WalkthroughTests/testAProxySetHereIsTriedAndSaved
"""
import base64, select, socket, sys, threading

LOG = open(sys.argv[1], "a", buffering=1)
def log(*a): LOG.write(" ".join(str(x) for x in a) + "\n")

def read_head(c):
    data = b""
    while b"\r\n\r\n" not in data:
        chunk = c.recv(4096)
        if not chunk: break
        data += chunk
    return data

def relay(c, host, port, name):
    try:
        up = socket.create_connection((host, port), timeout=10)
    except Exception as e:
        log(name, "upstream failed", host, port, repr(e)); return False
    up.settimeout(None); c.settimeout(None)
    def pump():
        socks = [c, up]
        try:
            while True:
                r, _, _ = select.select(socks, [], [], 60)
                if not r: break
                for s in r:
                    data = s.recv(65536)
                    if not data: return
                    (up if s is c else c).sendall(data)
        finally:
            up.close()
    return pump

def http_proxy(c, name, auth):
    head = read_head(c)
    first = head.split(b"\r\n")[0].decode("latin1")
    log(name, "request", first)
    if auth:
        want = b"proxy-authorization: basic " + base64.b64encode(auth.encode())
        if want.lower() not in head.lower():
            log(name, "407")
            c.sendall(b"HTTP/1.1 407 Proxy Authentication Required\r\nProxy-Authenticate: Basic realm=\"test\"\r\nContent-Length: 0\r\nConnection: close\r\n\r\n")
            return
    parts = first.split()
    if len(parts) >= 2 and parts[0].upper() == "CONNECT":
        host, _, port = parts[1].rpartition(":")
        pump = relay(c, host.strip("[]"), int(port), name)
        if not pump:
            c.sendall(b"HTTP/1.1 502 Bad Gateway\r\nContent-Length: 0\r\n\r\n"); return
        c.sendall(b"HTTP/1.1 200 Connection established\r\n\r\n")
        log(name, "tunnel", parts[1])
        pump()
    else:
        c.sendall(b"HTTP/1.1 200 OK\r\nContent-Length: 0\r\nConnection: close\r\n\r\n")

def recvn(c, n):
    data = b""
    while len(data) < n:
        chunk = c.recv(n - len(data))
        if not chunk: raise EOFError
        data += chunk
    return data

def socks_proxy(c, name, auth):
    ver, n = recvn(c, 2)
    methods = recvn(c, n)
    if auth:
        if 2 not in methods:
            log(name, "no user/password offered"); c.sendall(b"\x05\xff"); return
        c.sendall(b"\x05\x02")
        _, ulen = recvn(c, 2); user = recvn(c, ulen); plen = recvn(c, 1)[0]; pw = recvn(c, plen)
        ok = (user + b":" + pw).decode() == auth
        log(name, "auth", "ok" if ok else "refused")
        c.sendall(b"\x01\x00" if ok else b"\x01\x01")
        if not ok: return
    else:
        c.sendall(b"\x05\x00")
    ver, cmd, _, atyp = recvn(c, 4)
    if atyp == 1: addr = socket.inet_ntoa(recvn(c, 4))
    elif atyp == 3: addr = recvn(c, recvn(c, 1)[0]).decode()
    else: addr = socket.inet_ntop(socket.AF_INET6, recvn(c, 16))
    port = int.from_bytes(recvn(c, 2), "big")
    pump = relay(c, addr, port, name)
    if not pump:
        c.sendall(b"\x05\x05\x00\x01\x00\x00\x00\x00\x00\x00"); return
    c.sendall(b"\x05\x00\x00\x01\x00\x00\x00\x00\x00\x00")
    log(name, "tunnel", addr, port)
    pump()

def listen(port, name, handler, auth):
    s = socket.socket(); s.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
    s.bind(("127.0.0.1", port)); s.listen(16)
    def loop():
        while True:
            c, _ = s.accept()
            def run(c=c):
                try: handler(c, name, auth)
                except Exception as e: log(name, "error", repr(e))
                finally: c.close()
            threading.Thread(target=run, daemon=True).start()
    threading.Thread(target=loop, daemon=True).start()

base = int(sys.argv[2])
listen(base, "http", http_proxy, None)
listen(base + 1, "http-auth", http_proxy, "me:pw")
listen(base + 2, "socks5", socks_proxy, None)
listen(base + 3, "socks5-auth", socks_proxy, "me:pw")
log("listening", base)
threading.Event().wait()
