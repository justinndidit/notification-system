/*
  Each service now owns only the models it uses.

  Both services previously carried a byte-identical schema defining all four
  models, so every database materialised tables its service never touched. That
  is not merely untidy: renaming User.push_token broke template-service, which
  held a copy of a User model it has never read.

  user_service_db keeps User and Preference. The template tables here were
  always empty — template-service writes to its own database.
*/
DROP TABLE IF EXISTS "TemplateVersion";
DROP TABLE IF EXISTS "Template";
DROP TYPE IF EXISTS "NotificationChannel";
