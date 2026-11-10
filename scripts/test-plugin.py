"""Exercise the actual C ABI without a provider account or inference network call."""
import ctypes
import datetime
import json
import os
from pathlib import Path
import sys
import subprocess
from contextlib import nullcontext
import tempfile

if "PROXCLI_TEST_DATA" not in os.environ:
    with tempfile.TemporaryDirectory(prefix="proxcli-plugin-test-") as data:
        env = os.environ | {"PROXCLI_DATA_DIR": data, "PROXCLI_TEST_DATA": data}
        subprocess.run([sys.executable, __file__, sys.argv[1]], env=env, check=True)
    sys.exit(0)

with nullcontext(os.environ["PROXCLI_TEST_DATA"]) as data:
    library = ctypes.CDLL(str(Path(sys.argv[1]).resolve()))

    class Buffer(ctypes.Structure):
        _fields_ = [("ptr", ctypes.c_void_p), ("length", ctypes.c_size_t)]

    class Host(ctypes.Structure):
        _fields_ = [("abi", ctypes.c_uint32), ("ctx", ctypes.c_void_p), ("call", ctypes.c_void_p), ("free", ctypes.c_void_p)]

    class Plugin(ctypes.Structure):
        _fields_ = [("abi", ctypes.c_uint32), ("call", ctypes.c_void_p), ("free", ctypes.c_void_p), ("shutdown", ctypes.c_void_p)]

    plugin = Plugin()
    library.cliproxy_plugin_init.argtypes = [ctypes.POINTER(Host), ctypes.POINTER(Plugin)]
    assert library.cliproxy_plugin_init(ctypes.byref(Host(1, None, None, None)), ctypes.byref(plugin)) == 0
    call = ctypes.CFUNCTYPE(ctypes.c_int, ctypes.c_char_p, ctypes.c_void_p, ctypes.c_size_t, ctypes.POINTER(Buffer))(plugin.call)
    free = ctypes.CFUNCTYPE(None, ctypes.c_void_p, ctypes.c_size_t)(plugin.free)
    shutdown = ctypes.CFUNCTYPE(None)(plugin.shutdown)

    def invoke(method, body):
        encoded = json.dumps(body).encode()
        request = ctypes.create_string_buffer(encoded)
        response = Buffer()
        status = call(method.encode(), request, len(encoded), ctypes.byref(response))
        try:
            result = json.loads(ctypes.string_at(response.ptr, response.length))
            assert status == 0 and result["ok"]
            return result["result"]
        finally:
            free(response.ptr, response.length)

    shutdown()  # Probe lifecycle: init -> shutdown without registration must return immediately.
    registration = invoke("plugin.register", {})
    assert registration["capabilities"] == {"usage_plugin": True}
    now = datetime.datetime.now(datetime.timezone.utc)
    invoke("usage.handle", {
        "RequestID": "native-abi-test", "AuthID": "account-a", "Provider": "claude", "Model": "sonnet",
        "APIKey": "sensitive-client-key", "SessionID": "sensitive-session-id", "RequestedAt": now.isoformat(),
        "Detail": {"InputTokens": 10, "OutputTokens": 20, "CacheReadTokens": 30, "CacheCreationTokens": 40, "TotalTokens": 70},
        "Failure": {"Body": "sensitive-provider-error"},
        "ResponseHeaders": {"Set-Cookie": ["sensitive-cookie"], "Anthropic-Ratelimit-Unified-5h-Utilization": ["0.5"]}
    })
    shutdown()
    invoke("plugin.register", {})
    invoke("usage.handle", {"RequestID": "native-abi-reload", "AuthID": "account-a", "Provider": "claude", "Model": "sonnet", "RequestedAt": now.isoformat(), "Detail": {"InputTokens": 5}})
    shutdown()
    files = list((Path(data) / "telemetry").glob("*.json*"))
    assert files
    serialized = "".join(path.read_text() for path in files)
    for secret in ["sensitive-client-key", "sensitive-session-id", "sensitive-provider-error", "sensitive-cookie"]:
        assert secret not in serialized, "native observer persisted a secret"
    records = [json.loads(line) for path in files if path.name.startswith("usage-") and path.suffix == ".jsonl" for line in path.read_text().splitlines()]
    assert len(records) == 2
    assert any(record["usage"]["id"] == "native-abi-reload" for record in records)
    assert records[0]["usage"]["tokens"]["cacheWrite"] == 40
    assert json.loads((Path(data) / "telemetry/usage-status.json").read_text())["dropped"] == 0
    print("Native C ABI: registration, sanitized usage, quota headers, cache accounting and shutdown flush passed.")
