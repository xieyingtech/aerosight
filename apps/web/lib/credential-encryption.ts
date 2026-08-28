import {
  createCipheriv,
  createDecipheriv,
  createHash,
  createHmac,
  randomBytes
} from "node:crypto";

const HKDF_SALT = Buffer.from("aerosight/credential-encryption/salt/v1", "utf8");
const HKDF_INFO = Buffer.from("aerosight/credential-encryption/v1", "utf8");
const NONCE_BYTES = 12;

export type CredentialEnvelope = {
  version: 1;
  algorithm: "AES-256-GCM";
  keyFingerprint: string;
  nonce: string;
  ciphertext: string;
  authenticationTag: string;
};

function base64Url(value: Buffer) {
  return value.toString("base64url");
}

function fromBase64Url(value: string, field: string) {
  try {
    const decoded = Buffer.from(value, "base64url");
    if (decoded.length === 0) throw new Error("empty");
    return decoded;
  } catch {
    throw new Error(`CREDENTIAL_ENVELOPE_${field.toUpperCase()}_INVALID`);
  }
}

export function deriveCredentialKey(authSecret: string) {
  if (!authSecret) throw new Error("AUTH_SECRET_REQUIRED_FOR_CREDENTIALS");
  const pseudorandomKey = createHmac("sha256", HKDF_SALT).update(authSecret, "utf8").digest();
  return createHmac("sha256", pseudorandomKey)
    .update(Buffer.concat([HKDF_INFO, Buffer.from([1])]))
    .digest()
    .subarray(0, 32);
}

export function credentialKeyFingerprint(authSecret: string) {
  return createHash("sha256").update(deriveCredentialKey(authSecret)).digest("hex").slice(0, 16);
}

export function credentialAAD(resourceType: string, resourceId: string | number, scopeId?: string | number) {
  const normalizedType = resourceType.trim();
  const normalizedId = String(resourceId).trim();
  const normalizedScope = scopeId === undefined ? "platform" : String(scopeId).trim();
  if (!normalizedType || !normalizedId || !normalizedScope) throw new Error("CREDENTIAL_AAD_INVALID");
  return `aerosight:${normalizedType}:${normalizedScope}:${normalizedId}`;
}

export function encryptCredentialBytes(
  plaintext: Buffer,
  authSecret: string,
  aad: string,
  nonce = randomBytes(NONCE_BYTES)
): CredentialEnvelope {
  if (nonce.length !== NONCE_BYTES) throw new Error("CREDENTIAL_NONCE_INVALID");
  if (!aad) throw new Error("CREDENTIAL_AAD_INVALID");
  const key = deriveCredentialKey(authSecret);
  const cipher = createCipheriv("aes-256-gcm", key, nonce);
  cipher.setAAD(Buffer.from(aad, "utf8"));
  const ciphertext = Buffer.concat([cipher.update(plaintext), cipher.final()]);
  return {
    version: 1,
    algorithm: "AES-256-GCM",
    keyFingerprint: createHash("sha256").update(key).digest("hex").slice(0, 16),
    nonce: base64Url(nonce),
    ciphertext: base64Url(ciphertext),
    authenticationTag: base64Url(cipher.getAuthTag())
  };
}

export function decryptCredentialBytes(envelope: CredentialEnvelope, authSecret: string, aad: string) {
  if (envelope.version !== 1 || envelope.algorithm !== "AES-256-GCM") {
    throw new Error("CREDENTIAL_ENVELOPE_VERSION_UNSUPPORTED");
  }
  const key = deriveCredentialKey(authSecret);
  const fingerprint = createHash("sha256").update(key).digest("hex").slice(0, 16);
  if (envelope.keyFingerprint !== fingerprint) throw new Error("CREDENTIAL_KEY_MISMATCH");
  const nonce = fromBase64Url(envelope.nonce, "nonce");
  if (nonce.length !== NONCE_BYTES) throw new Error("CREDENTIAL_ENVELOPE_NONCE_INVALID");
  const decipher = createDecipheriv("aes-256-gcm", key, nonce);
  decipher.setAAD(Buffer.from(aad, "utf8"));
  decipher.setAuthTag(fromBase64Url(envelope.authenticationTag, "authentication_tag"));
  try {
    return Buffer.concat([
      decipher.update(fromBase64Url(envelope.ciphertext, "ciphertext")),
      decipher.final()
    ]);
  } catch {
    throw new Error("CREDENTIAL_DECRYPTION_FAILED");
  }
}

export function encryptCredentialObject(
  credentials: Record<string, unknown>,
  authSecret: string,
  aad: string,
  nonce?: Buffer
) {
  return encryptCredentialBytes(Buffer.from(JSON.stringify(credentials), "utf8"), authSecret, aad, nonce);
}

export function decryptCredentialObject<T extends Record<string, unknown>>(
  envelope: CredentialEnvelope,
  authSecret: string,
  aad: string
) {
  const plaintext = decryptCredentialBytes(envelope, authSecret, aad);
  try {
    const parsed = JSON.parse(plaintext.toString("utf8"));
    if (!parsed || typeof parsed !== "object" || Array.isArray(parsed)) throw new Error("not object");
    return parsed as T;
  } catch {
    throw new Error("CREDENTIAL_PAYLOAD_INVALID");
  } finally {
    plaintext.fill(0);
  }
}
