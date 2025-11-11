/*
  Warnings:

  - Changed the type of `event` on the `Template` table. No cast exists, the column would be dropped and recreated, which cannot be done if there is data, since the column is required.

*/
-- AlterTable
ALTER TABLE "Template" DROP COLUMN "event",
ADD COLUMN     "event" TEXT NOT NULL;

-- DropEnum
DROP TYPE "TemplateEvent";

-- CreateIndex
CREATE INDEX "Template_event_language_idx" ON "Template"("event", "language");

-- CreateIndex
CREATE UNIQUE INDEX "Template_event_language_key" ON "Template"("event", "language");
