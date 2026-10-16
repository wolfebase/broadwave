import { test } from "node:test";
import assert from "node:assert/strict";
import { checkAddresses, privateIPv4, privateIPv6, canonicalIPv6 } from "../src/ip.ts";

test("home IPv4 ranges pass and public ones do not", () => {
  for (const a of ["10.0.0.1", "10.255.255.255", "172.16.0.1", "172.31.255.254", "192.168.1.2", "100.64.0.1", "100.127.255.255"]) {
    assert.equal(privateIPv4(a), true, a);
  }
  for (const a of ["8.8.8.8", "172.15.0.1", "172.32.0.1", "192.169.0.1", "100.63.255.255", "100.128.0.1", "127.0.0.1", "0.0.0.0", "169.254.1.1", "1.2.3", "010.0.0.1", "10.0.0.256", "10.0.0.1 ", "224.0.0.1"]) {
    assert.equal(privateIPv4(a), false, a);
  }
});

test("only unique local IPv6 passes", () => {
  assert.equal(privateIPv6("fd12:3456:789a::1"), true);
  assert.equal(privateIPv6("FD00::"), true);
  for (const a of ["fc00::1", "fe80::1", "2001:db8::1", "::1", "::", "::ffff:192.168.1.2", "fd00::1::2", "fd00:1:2:3:4:5:6:7:8", "fd00:12345::1"]) {
    assert.equal(privateIPv6(a), false, a);
  }
  assert.equal(canonicalIPv6("fd00::1"), "fd00:0:0:0:0:0:0:1");
});

test("address lists are checked and deduplicated", () => {
  assert.deepEqual(checkAddresses({ ipv4: ["192.168.1.2", "192.168.1.2"], ipv6: ["fd00::1"] }), {
    ipv4: ["192.168.1.2"],
    ipv6: ["fd00:0:0:0:0:0:0:1"],
  });
  assert.equal(typeof checkAddresses({ ipv4: ["192.168.1.2", "8.8.8.8"] }), "string");
  assert.equal(typeof checkAddresses({ ipv4: [] }), "string");
  assert.equal(typeof checkAddresses({ ipv4: ["10.0.0.1", "10.0.0.2", "10.0.0.3", "10.0.0.4", "10.0.0.5"] }), "string");
  assert.equal(typeof checkAddresses({ ipv4: "10.0.0.1" }), "string");
  assert.equal(typeof checkAddresses(null), "string");
  assert.equal(typeof checkAddresses({ ipv4: [10] }), "string");
});
