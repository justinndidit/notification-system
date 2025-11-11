/*
  Warnings:

  - The `push_token` column on the `User` table would be dropped and recreated. This will lead to data loss if there is data in the column.

*/
-- DropIndex
DROP INDEX "User_push_token_key";

-- AlterTable
ALTER TABLE "User" DROP COLUMN "push_token",
ADD COLUMN     "push_token" JSONB;
