import { Test, TestingModule } from '@nestjs/testing';
import { JwtService } from '@nestjs/jwt';
import { ConflictException, NotFoundException } from '@nestjs/common';
import * as bcrypt from 'bcrypt';
import { UserService } from './user.service';
import { PrismaService } from 'src/prisma/prisma.service';
import { CacheService } from '../common/cache.service';

/**
 * Minimal Prisma double. Records what the service asked for so assertions can
 * be made about the data written, not only the value returned.
 */
const buildPrisma = () => {
  const created: Record<string, unknown>[] = [];

  const user = {
    findUnique: jest.fn(),
    create: jest.fn((args: { data: Record<string, unknown> }) => {
      created.push(args.data);
      return Promise.resolve({
        id: 'user-1',
        role: 'user',
        ...args.data,
      });
    }),
    update: jest.fn((args: { data: Record<string, unknown> }) =>
      Promise.resolve({ id: 'user-1', ...args.data }),
    ),
  };

  const preference = { create: jest.fn().mockResolvedValue({}) };

  return {
    created,
    user,
    preference,
    $transaction: jest.fn((fn: (tx: unknown) => unknown) =>
      Promise.resolve(fn({ user, preference })),
    ),
  };
};

describe('UserService', () => {
  let service: UserService;
  let prisma: ReturnType<typeof buildPrisma>;
  let cache: { invalidateUser: jest.Mock; get: jest.Mock; set: jest.Mock };

  beforeEach(async () => {
    prisma = buildPrisma();
    cache = {
      invalidateUser: jest.fn().mockResolvedValue(undefined),
      get: jest.fn().mockResolvedValue(null),
      set: jest.fn().mockResolvedValue(undefined),
    };

    const module: TestingModule = await Test.createTestingModule({
      providers: [
        UserService,
        { provide: PrismaService, useValue: prisma },
        { provide: JwtService, useValue: { sign: jest.fn(() => 'signed-token') } },
        { provide: CacheService, useValue: cache },
      ],
    }).compile();

    service = module.get<UserService>(UserService);
  });

  describe('signup', () => {
    it('never takes the role from the request body', async () => {
      prisma.user.findUnique.mockResolvedValue(null);

      // A caller trying to escalate. This exact payload used to create an admin.
      await service.signup({
        name: 'Mallory',
        email: 'mallory@example.com',
        password: 'password123',
        role: 'admin',
      } as never);

      expect(prisma.created).toHaveLength(1);
      expect(prisma.created[0].role).toBe('user');
    });

    it('hashes the password rather than storing it', async () => {
      prisma.user.findUnique.mockResolvedValue(null);

      await service.signup({
        name: 'Ada',
        email: 'ada@example.com',
        password: 'password123',
      } as never);

      const stored = prisma.created[0].password as string;
      expect(stored).not.toBe('password123');
      await expect(bcrypt.compare('password123', stored)).resolves.toBe(true);
    });

    it('creates a preference row alongside the user', async () => {
      prisma.user.findUnique.mockResolvedValue(null);

      await service.signup({
        name: 'Ada',
        email: 'ada@example.com',
        password: 'password123',
      } as never);

      expect(prisma.preference.create).toHaveBeenCalledTimes(1);
    });

    it('does not return the password hash', async () => {
      prisma.user.findUnique.mockResolvedValue(null);

      const result = await service.signup({
        name: 'Ada',
        email: 'ada@example.com',
        password: 'password123',
      } as never);

      expect(result.user).not.toHaveProperty('password');
    });

    it('rejects an email that already exists', async () => {
      prisma.user.findUnique.mockResolvedValue({ id: 'existing' });

      await expect(
        service.signup({
          name: 'Ada',
          email: 'taken@example.com',
          password: 'password123',
        } as never),
      ).rejects.toBeInstanceOf(ConflictException);
    });
  });

  describe('signin', () => {
    it('issues a token when the password matches', async () => {
      prisma.user.findUnique.mockResolvedValue({
        id: 'user-1',
        email: 'ada@example.com',
        role: 'user',
        password: await bcrypt.hash('password123', 10),
      });

      const result = await service.signin({
        email: 'ada@example.com',
        password: 'password123',
      } as never);

      expect(result.token).toBe('signed-token');
      expect(result).not.toHaveProperty('password');
    });

    it('gives the same error for an unknown email and a wrong password', async () => {
      // Distinguishing the two tells an attacker which emails are registered.
      prisma.user.findUnique.mockResolvedValue(null);
      const unknownEmail = await service
        .signin({ email: 'nobody@example.com', password: 'password123' } as never)
        .catch((e: Error) => e.message);

      prisma.user.findUnique.mockResolvedValue({
        id: 'user-1',
        email: 'ada@example.com',
        role: 'user',
        password: await bcrypt.hash('the-real-password', 10),
      });
      const wrongPassword = await service
        .signin({ email: 'ada@example.com', password: 'password123' } as never)
        .catch((e: Error) => e.message);

      expect(unknownEmail).toBe(wrongPassword);
    });
  });

  describe('updateRole', () => {
    it('updates the role and invalidates the cache', async () => {
      prisma.user.findUnique.mockResolvedValue({ id: 'user-1' });

      const result = await service.updateRole('user-1', 'admin');

      expect(result.role).toBe('admin');
      expect(cache.invalidateUser).toHaveBeenCalledWith('user-1');
    });

    it('rejects an unknown user', async () => {
      prisma.user.findUnique.mockResolvedValue(null);

      await expect(service.updateRole('missing', 'admin')).rejects.toBeInstanceOf(
        NotFoundException,
      );
    });
  });

  describe('getDeliveryProfile', () => {
    it('reports consent and contact details for the orchestrator', async () => {
      prisma.user.findUnique.mockResolvedValue({
        id: 'user-1',
        name: 'Ada',
        email: 'ada@example.com',
        device_tokens: [{ token: 'fcm-1', platform: 'android' }],
        preferences: {
          email_opt_in: false,
          push_opt_in: true,
          daily_limit: 50,
          language: 'fr',
        },
      });

      const profile = await service.getDeliveryProfile('user-1');

      expect(profile.email).toBe('ada@example.com');
      expect(profile.email_opt_in).toBe(false);
      expect(profile.push_opt_in).toBe(true);
      expect(profile.device_tokens).toHaveLength(1);
      expect(profile.language).toBe('fr');
    });

    it('defaults to opted in when no preference row exists', async () => {
      prisma.user.findUnique.mockResolvedValue({
        id: 'user-2',
        name: 'Grace',
        email: 'grace@example.com',
        device_tokens: null,
        preferences: null,
      });

      const profile = await service.getDeliveryProfile('user-2');

      expect(profile.email_opt_in).toBe(true);
      expect(profile.device_tokens).toEqual([]);
    });
  });

  describe('updateDeviceTokens', () => {
    it('de-duplicates tokens', async () => {
      prisma.user.findUnique.mockResolvedValue({ id: 'user-1' });

      await service.updateDeviceTokens('user-1', [
        { token: 'a', platform: 'android' },
        { token: 'a', platform: 'android' },
        { token: 'b', platform: 'ios' },
      ] as never);

      const written = prisma.user.update.mock.calls[0][0].data
        .device_tokens as unknown[];
      expect(written).toHaveLength(2);
    });
  });
});
