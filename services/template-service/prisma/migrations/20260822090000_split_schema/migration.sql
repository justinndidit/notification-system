/*
  Drops the user tables template-service never used.

  Its only reference to prisma.user was a test asserting the render path does
  *not* consult it — recipient resolution belongs to the orchestrator.
*/
DROP TABLE IF EXISTS "Preference";
DROP TABLE IF EXISTS "User";
