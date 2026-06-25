#!/usr/bin/env python3
"""Sim test: Home → MoveTo → Stop → Jog via WebSocket."""
import json, time, uuid, sys
import websocket

URL = "ws://localhost:8080/ws"
PASS = "\033[32mPASS\033[0m"
FAIL = "\033[31mFAIL\033[0m"

def send_cmd(ws, cmd, **kw):
    msg = {"id": str(uuid.uuid4()), "cmd": cmd, **kw}
    ws.send(json.dumps(msg))
    return msg["id"]

def wait_result(ws, cmd_id, timeout=15):
    ws.settimeout(timeout)
    events = []
    while True:
        raw = ws.recv()
        ev = json.loads(raw)
        if ev.get("id") != cmd_id:
            continue  # status broadcast or other command
        events.append(ev)
        if ev["kind"] in ("done", "error", "status"):
            return ev, events

def check(label, ev, expect_kind="done"):
    ok = ev["kind"] == expect_kind
    mark = PASS if ok else FAIL
    msg = ev.get("message", ev.get("payload", ""))
    print(f"  {mark}  {label}: {msg}")
    if not ok:
        print(f"        full event: {ev}")
    return ok

ws = websocket.create_connection(URL)
print(f"Connected to {URL}\n")

all_ok = True

# ── 1. Status broadcast arrives ───────────────────────────────────────────────
print("1. Status broadcast")
ws.settimeout(3)
try:
    raw = ws.recv()
    st = json.loads(raw)
    ok = st.get("kind") == "status" and "payload" in st
    mark = PASS if ok else FAIL
    payload = st.get("payload", {})
    print(f"  {mark}  Got status: homed={payload.get('homed')} busy={payload.get('busy')} x={payload.get('x')} y={payload.get('y')}")
    if not ok:
        all_ok = False
except Exception as e:
    print(f"  {FAIL}  No status broadcast: {e}")
    all_ok = False

# ── 2. Home ───────────────────────────────────────────────────────────────────
print("\n2. Home (homing takes ~1s per motor in sim)")
t0 = time.time()
cid = send_cmd(ws, "home")
ev, events = wait_result(ws, cid, timeout=20)
elapsed = time.time() - t0
ok = check(f"Home ({elapsed:.1f}s)", ev)
all_ok = all_ok and ok

# Check position declared at center
if ok:
    payload = ev.get("payload") or {}
    msg = ev.get("message", "")
    print(f"         message: {msg}")

# ── 3. Status after home — homed=true ─────────────────────────────────────────
print("\n3. Status after home")
cid = send_cmd(ws, "status")
ev, _ = wait_result(ws, cid)
payload = ev.get("payload", {})
ok = payload.get("homed") == True
mark = PASS if ok else FAIL
print(f"  {mark}  homed={payload.get('homed')} x={payload.get('x', 0):.0f} y={payload.get('y', 0):.0f}")
all_ok = all_ok and ok

# ── 4. MoveTo (350, 600) ──────────────────────────────────────────────────────
print("\n4. MoveTo(350, 600) at 50 mm/s")
t0 = time.time()
cid = send_cmd(ws, "move", x=350, y=600, speed=50)
ev, _ = wait_result(ws, cid, timeout=30)
elapsed = time.time() - t0
ok = check(f"MoveTo ({elapsed:.1f}s)", ev)
all_ok = all_ok and ok

# ── 5. Status — position updated ──────────────────────────────────────────────
print("\n5. Status after MoveTo")
cid = send_cmd(ws, "status")
ev, _ = wait_result(ws, cid)
payload = ev.get("payload", {})
x, y = payload.get("x", -1), payload.get("y", -1)
ok = abs(x - 350) < 10 and abs(y - 600) < 10
mark = PASS if ok else FAIL
print(f"  {mark}  x={x:.1f} y={y:.1f} (want 350, 600)")
all_ok = all_ok and ok

# ── 6. MoveTo back to center ──────────────────────────────────────────────────
print("\n6. MoveTo(700, 1200) — back to center")
t0 = time.time()
cid = send_cmd(ws, "move", x=700, y=1200, speed=50)
ev, _ = wait_result(ws, cid, timeout=30)
elapsed = time.time() - t0
ok = check(f"MoveTo center ({elapsed:.1f}s)", ev)
all_ok = all_ok and ok

# ── 7. Stop while idle (should be ok) ────────────────────────────────────────
print("\n7. Stop (no active operation)")
cid = send_cmd(ws, "stop")
ev, _ = wait_result(ws, cid)
ok = check("Stop idle", ev)
all_ok = all_ok and ok

# ── 8. Jog motor 1 then stop ─────────────────────────────────────────────────
print("\n8. Jog motor 1 at 25 RPM → wait 200ms → jog_stop")
cid = send_cmd(ws, "jog_start", motor=1, rpm=25)
ev, _ = wait_result(ws, cid)
ok = check("jog_start M1", ev)
all_ok = all_ok and ok

time.sleep(0.2)

cid = send_cmd(ws, "jog_stop", motor=1)
ev, _ = wait_result(ws, cid)
ok = check("jog_stop M1", ev)
all_ok = all_ok and ok

# ── 9. Read motor status (FC04) ───────────────────────────────────────────────
print("\n9. read_motor_status motor=2")
cid = send_cmd(ws, "read_motor_status", motor=2)
ev, _ = wait_result(ws, cid)
ok = ev["kind"] == "done" and isinstance(ev.get("payload"), dict)
mark = PASS if ok else FAIL
payload = ev.get("payload", {})
print(f"  {mark}  speed_rpm={payload.get('speed_rpm')} torque_pct={payload.get('torque_pct')} bus_voltage_v={payload.get('bus_voltage_v')} fault_code={payload.get('fault_code')}")
all_ok = all_ok and ok

# ── 10. Read param P-137 ──────────────────────────────────────────────────────
print("\n10. read_param motor=1 addr=137 (P-137 speed setpoint)")
cid = send_cmd(ws, "read_param", motor=1, addr=137)
ev, _ = wait_result(ws, cid)
ok = ev["kind"] == "done"
mark = PASS if ok else FAIL
print(f"  {mark}  {ev.get('payload')}")
all_ok = all_ok and ok

ws.close()

print(f"\n{'='*40}")
print(f"  Result: {'ALL PASS ✓' if all_ok else 'SOME FAILURES ✗'}")
print(f"{'='*40}")
sys.exit(0 if all_ok else 1)
