/*
  Replaces the single Web Push subscription blob with an array of FCM device
  tokens, matching what the push service actually sends through FCM HTTP v1.

  Existing values are W3C Web Push subscriptions ({endpoint, keys}). They cannot
  be converted into FCM registration tokens — they address a different delivery
  mechanism — so they are discarded and clients must re-register.

  Expected shape: [{ "token": "...", "platform": "android" | "ios" }]
*/
-- AlterTable
ALTER TABLE "User" RENAME COLUMN "push_token" TO "device_tokens";

-- Discard unconvertible Web Push subscriptions
UPDATE "User" SET "device_tokens" = NULL WHERE "device_tokens" IS NOT NULL;
