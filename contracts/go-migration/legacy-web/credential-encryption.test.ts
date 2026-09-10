import assert from "node:assert/strict";
import test from "node:test";

import {
  credentialAAD,
  credentialKeyFingerprint,
  decryptCredentialObject,
  encryptCredentialObject
} from "./credential-encryption.ts";

const authSecret = "0123456789abcdef0123456789abcdef";
const aad = credentialAAD("algorithm-provider", 17, 3);
const nonce = Buffer.from("000102030405060708090a0b", "hex");

test("credential encryption round-trips and uses a fresh nonce by default", () => {
  const first = encryptCredentialObject({ token: "secret-token" }, authSecret, aad);
  const second = encryptCredentialObject({ token: "secret-token" }, authSecret, aad);
  assert.notEqual(first.nonce, second.nonce);
  assert.deepEqual(decryptCredentialObject(first, authSecret, aad), { token: "secret-token" });
});

test("credential envelope rejects the wrong scope and auth secret", () => {
  const envelope = encryptCredentialObject({ token: "secret-token" }, authSecret, aad);
  assert.throws(
    () => decryptCredentialObject(envelope, authSecret, credentialAAD("algorithm-provider", 17, 4)),
    /CREDENTIAL_DECRYPTION_FAILED/
  );
  assert.throws(
    () => decryptCredentialObject(envelope, "fedcba9876543210fedcba9876543210", aad),
    /CREDENTIAL_KEY_MISMATCH/
  );
});

test("credential encryption matches the shared Go test vector", () => {
  const envelope = encryptCredentialObject(
    { password: "secret", username: "pilot" },
    authSecret,
    aad,
    nonce
  );
  assert.deepEqual(envelope, {
    version: 1,
    algorithm: "AES-256-GCM",
    keyFingerprint: credentialKeyFingerprint(authSecret),
    nonce: "AAECAwQFBgcICQoL",
    ciphertext: "HXWy7eF_4k6LAqvbh-dL7wVmb05NgxRR52mnDf_6eMToeCL6qRQbcQ",
    authenticationTag: "WUHcFG7rJk8ARwdWzHUffQ"
  });
});
