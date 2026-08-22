/**
 * Creates the first admin.
 *
 * Removing `role` from the signup payload closed a privilege-escalation hole,
 * but it also left no way to create an admin at all: PATCH /user/:id/role
 * itself requires one. This is the bootstrap that breaks that circle.
 *
 * Run once against a fresh database:
 *
 *   ADMIN_EMAIL=admin@example.com ADMIN_PASSWORD='...' pnpm seed:admin
 *
 * Idempotent: an existing user with that email is promoted rather than
 * duplicated, so it is safe to re-run.
 */
import { PrismaClient } from '@prisma/client';
import * as bcrypt from 'bcrypt';

const prisma = new PrismaClient();

const MIN_PASSWORD_LENGTH = 12;

async function main() {
  const email = process.env.ADMIN_EMAIL;
  const password = process.env.ADMIN_PASSWORD;
  const name = process.env.ADMIN_NAME ?? 'Administrator';

  if (!email || !password) {
    throw new Error(
      'ADMIN_EMAIL and ADMIN_PASSWORD must both be set.\n' +
        "Example: ADMIN_EMAIL=admin@example.com ADMIN_PASSWORD='...' pnpm seed:admin",
    );
  }

  // Deliberately stricter than the signup rule. This account can create
  // templates and read every user's preferences.
  if (password.length < MIN_PASSWORD_LENGTH) {
    throw new Error(
      `ADMIN_PASSWORD must be at least ${MIN_PASSWORD_LENGTH} characters.`,
    );
  }

  const existing = await prisma.user.findUnique({ where: { email } });

  if (existing) {
    if (existing.role === 'admin') {
      console.log(`✓ ${email} is already an admin (${existing.id})`);
      return;
    }

    const promoted = await prisma.user.update({
      where: { email },
      data: { role: 'admin' },
      select: { id: true, email: true, role: true },
    });
    console.log(`✓ Promoted existing user to admin: ${promoted.email} (${promoted.id})`);
    return;
  }

  const hashed = await bcrypt.hash(password, 10);

  const admin = await prisma.$transaction(async (tx) => {
    const user = await tx.user.create({
      data: { name, email, password: hashed, role: 'admin' },
      select: { id: true, email: true, role: true },
    });

    // Signup creates this alongside the user; do the same here so the admin has
    // a complete profile and can receive notifications like anyone else.
    await tx.preference.create({ data: { user_id: user.id } });

    return user;
  });

  console.log(`✓ Created admin: ${admin.email} (${admin.id})`);
}

main()
  .catch((error: unknown) => {
    console.error(
      `✗ ${error instanceof Error ? error.message : String(error)}`,
    );
    process.exitCode = 1;
  })
  .finally(() => {
    void prisma.$disconnect();
  });
