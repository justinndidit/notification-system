/* eslint-disable @typescript-eslint/no-unused-vars */
import {
  BadRequestException,
  ConflictException,
  Injectable,
  NotFoundException,
  UnauthorizedException,
} from '@nestjs/common';
import { JwtService } from '@nestjs/jwt';
import { PrismaService } from 'src/prisma/prisma.service';
import {
  LoginDto,
  PaginationDto,
  RegisterDto,
  UpdatePreferenceDto,
} from './dto/user.dto';
import * as bcrypt from 'bcrypt';
import { Preference, Prisma, User } from '@notification/user-prisma';
import { CacheService } from '../common/cache.service';

@Injectable()
export class UserService {
  constructor(
    private prisma: PrismaService,
    private jwtService: JwtService,
    private cacheService: CacheService,
  ) {}

  //SIGN UP
  async signup(registerDto: RegisterDto) {
    const { name, email, password } = registerDto;
    //checking if user already exists
    const existingUser = await this.prisma.user.findUnique({
      where: { email },
    });
    if (existingUser) {
      throw new ConflictException('User already exists');
    }
    //hashing password
    const hashedPassword = await bcrypt.hash(password, 10);

    const user = await this.prisma.$transaction(async (prisma) => {
      const newUser = await prisma.user.create({
        data: {
          name,
          email,
          password: hashedPassword,
          // Never taken from the request body — see UpdateRoleDto / updateRole().
          role: 'user',
        },
      });
      await prisma.preference.create({
        data: {
          user_id: newUser.id,
        },
      });
      return newUser;
    });

    const { password: _, ...userData } = user;

    return {
      user: userData,
    };
  }

  //SIGN IN
  async signin(loginDto: LoginDto) {
    const { email, password } = loginDto;
    //check if user exists
    const user = await this.prisma.user.findUnique({
      where: {
        email,
      },
    });
    if (!user) {
      throw new UnauthorizedException(
        'Email or password is incorrect, please provide a valid credientials',
      );
    }

    //check if password is correct
    const isPasswordValid = await bcrypt.compare(password, user.password);

    if (!isPasswordValid) {
      throw new UnauthorizedException(
        'Email or password is incorrect, please provide a valid credientials',
      );
    }
    const payload = { user_id: user.id, role: user.role };
    const token = this.jwtService.sign(payload, { expiresIn: '7d' });

    const { password: _, ...safeUser } = user;

    return { message: 'Signin successful', role: safeUser.role, token };
  }

  //GET ALL USER
  async getPaginatedUsers(
    paginationDto: PaginationDto,
  ): Promise<PaginatedResponse<Omit<User, 'password'>>> {
    const page = paginationDto.page ?? 1;
    const limit = paginationDto.limit ?? 10;
    const skip = (page - 1) * limit;
    const [total, users] = await Promise.all([
      this.prisma.user.count(),
      this.prisma.user.findMany({
        include: {
          preferences: true,
        },
        skip,
        take: limit,
        orderBy: {
          created_at: 'desc',
        },
      }),
    ]);

    const totalPages = Math.ceil(total / limit);

    const meta: PaginationMeta = {
      total,
      limit,
      page,
      total_pages: totalPages,
      has_next: page < totalPages,
      has_previous: page > 1,
    };

    if (!users) {
      throw new NotFoundException('Users not found');
    }
    if (users.length === 0 && page > 1) {
      throw new NotFoundException(`No users found on page ${page}`);
    }

    const safeUsers = users.map(({ password, ...user }) => user);
    return { data: safeUsers, meta };
  }

  //GET USER BY ID
  async getUserById(userId: string) {
    const cacheKey = `user:${userId}`;

    // Try to get from cache first
    const cachedUser = await this.cacheService.get<
      User & { preferences: Preference | null }
    >(cacheKey);
    if (cachedUser) {
      return cachedUser;
    }

    // If not in cache, fetch from database
    const user = await this.prisma.user.findUnique({
      where: { id: userId },
      include: {
        preferences: true,
      },
    });
    if (!user) {
      throw new NotFoundException('User not found');
    }

    // Cache for 10 minutes
    await this.cacheService.set(cacheKey, user, 600);

    return user;
  }

  //UPDATE PREFERENCE
  async updatePreference(userId: string, updateDto: UpdatePreferenceDto) {
    // Check if preference exists for user
    const existingPreference = await this.prisma.preference.findUnique({
      where: { user_id: userId },
    });

    if (!existingPreference) {
      throw new NotFoundException('Preferences not found for this user');
    }

    // Update only the provided fields
    const updatedPreference = await this.prisma.preference.update({
      where: { user_id: userId },
      data: {
        ...updateDto,
      },
    });

    // Invalidate cache
    await this.cacheService.invalidateUser(userId);

    return { message: 'Preference updated successfully', updatedPreference };
  }

  //GET ALL PREFERENCE
  async getPaginatedUserPreferences(
    paginationDto: PaginationDto,
  ): Promise<PaginatedResponse<Preference>> {
    const page = paginationDto.page ?? 1;
    const limit = paginationDto.limit ?? 10;
    const skip = (page - 1) * limit;
    const [total, preferences] = await Promise.all([
      this.prisma.preference.count(), // Or global if needed
      this.prisma.preference.findMany({
        skip,
        take: limit,
        orderBy: { updated_at: 'desc' },
      }),
    ]);

    if (!preferences) {
      throw new NotFoundException('Preferences not found');
    }

    if (preferences.length === 0 && page > 1) {
      throw new NotFoundException(`No users found on page ${page}`);
    }

    const totalPages = Math.ceil(total / limit);
    const meta: PaginationMeta = {
      total,
      limit,
      page,
      total_pages: totalPages,
      has_next: page < totalPages,
      has_previous: page > 1,
    };

    return { data: preferences, meta };
  }

  //GET users preference by Ids
  async getUserPreference(userId: string) {
    const cacheKey = `user:preferences:${userId}`;

    // Try to get from cache first
    const cachedPreferences = await this.cacheService.get<Preference | null>(
      cacheKey,
    );
    if (cachedPreferences !== null) {
      return cachedPreferences;
    }

    // If not in cache, fetch from database
    const user = await this.prisma.user.findUnique({
      where: { id: userId },
      include: {
        preferences: true,
      },
    });

    if (!user) {
      throw new NotFoundException('User not found');
    }

    // Cache for 10 minutes
    await this.cacheService.set(cacheKey, user.preferences, 600);

    return user.preferences;
  }

  //UPDATE DEVICE TOKENS
  // Replaces the full set for this user. FCM registration tokens rotate, so the
  // client owns the list and sends the current one on each registration.
  async updateDeviceTokens(
    userId: string,
    deviceTokens: DeviceToken[],
  ): Promise<{ user_id: string; device_tokens: DeviceToken[] }> {
    const user = await this.prisma.user.findUnique({ where: { id: userId } });
    if (!user) {
      throw new NotFoundException('User not found');
    }

    // De-duplicate on token value; one device should appear only once.
    const seen = new Set<string>();
    const deduped = deviceTokens.filter((dt) => {
      if (!dt.token || seen.has(dt.token)) return false;
      seen.add(dt.token);
      return true;
    });

    const updated = await this.prisma.user.update({
      where: { id: userId },
      data: {
        device_tokens: deduped as unknown as Prisma.InputJsonValue,
      },
      select: { id: true, device_tokens: true },
    });

    // Invalidate cache since user data changed
    await this.cacheService.invalidateUser(userId);

    return {
      user_id: updated.id,
      device_tokens: (updated.device_tokens as DeviceToken[] | null) ?? [],
    };
  }

  //DELIVERY PROFILE — everything the orchestrator needs to deliver to a user
  // in one call: where to reach them, and whether they've consented.
  async getDeliveryProfile(userId: string) {
    const cacheKey = `user:delivery-profile:${userId}`;

    const cached = await this.cacheService.get<DeliveryProfile>(cacheKey);
    if (cached !== null) {
      return cached;
    }

    const user = await this.prisma.user.findUnique({
      where: { id: userId },
      include: { preferences: true },
    });

    if (!user) {
      throw new NotFoundException('User not found');
    }

    const profile: DeliveryProfile = {
      user_id: user.id,
      name: user.name,
      email: user.email,
      device_tokens: (user.device_tokens as DeviceToken[] | null) ?? [],
      email_opt_in: user.preferences?.email_opt_in ?? true,
      push_opt_in: user.preferences?.push_opt_in ?? true,
      daily_limit: user.preferences?.daily_limit ?? 100,
      language: user.preferences?.language ?? 'en',
    };

    // Short TTL: consent changes should take effect quickly.
    await this.cacheService.set(cacheKey, profile, 300);

    return profile;
  }

  //UPDATE ROLE (admin only — authorization is enforced in the controller)
  async updateRole(
    userId: string,
    role: string,
  ): Promise<{ user_id: string; role: string }> {
    const user = await this.prisma.user.findUnique({ where: { id: userId } });
    if (!user) {
      throw new NotFoundException('User not found');
    }

    const updated = await this.prisma.user.update({
      where: { id: userId },
      data: { role },
      select: { id: true, role: true },
    });

    // Invalidate cache since user data changed
    await this.cacheService.invalidateUser(userId);

    return { user_id: updated.id, role: updated.role };
  }
}
