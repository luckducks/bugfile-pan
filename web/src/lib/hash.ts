import { createSHA256 } from "hash-wasm";

// SHA-256 -> Base62, matching BigFile's file hashing scheme.
//
// Verified against the API doc example: the bytes of "hello" hash to
// "Aeo7HbT3j4TvaA0SlueQYMNLP6S43jNjrIbYLeK5ySK".
//
// The digest is treated as a single big-endian unsigned integer and encoded
// with the alphabet 0-9A-Za-z. Leading zero bytes are not preserved, matching
// the reference bignum encoding used by the site.

const BASE62_ALPHABET =
  "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz";

export function base62Encode(bytes: Uint8Array): string {
  let num = 0n;
  for (const b of bytes) {
    num = (num << 8n) | BigInt(b);
  }
  if (num === 0n) return BASE62_ALPHABET[0];
  const base = 62n;
  let out = "";
  while (num > 0n) {
    const rem = Number(num % base);
    out = BASE62_ALPHABET[rem] + out;
    num /= base;
  }
  return out;
}

const HASH_READ_SIZE = 8 * 1024 * 1024;

// Hash incrementally so large files do not need to be loaded into memory in
// full. hash-wasm supplies the streaming state that Web Crypto does not expose.
export async function hashBlob(blob: Blob, signal?: AbortSignal): Promise<string> {
  const hasher = await createSHA256();
  hasher.init();
  for (let offset = 0; offset < blob.size; offset += HASH_READ_SIZE) {
    signal?.throwIfAborted();
    const chunk = blob.slice(offset, Math.min(offset + HASH_READ_SIZE, blob.size));
    hasher.update(new Uint8Array(await chunk.arrayBuffer()));
  }
  signal?.throwIfAborted();
  return base62Encode(hasher.digest("binary"));
}

export async function hashFile(file: File, signal?: AbortSignal): Promise<string> {
  return hashBlob(file, signal);
}
