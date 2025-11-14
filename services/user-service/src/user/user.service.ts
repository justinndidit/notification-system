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
import { Preference, Prisma, User } from '@prisma/client';
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
    const { name, email, password, push_token, role } = registerDto;
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
          push_token,
          role,
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

  //UPDATE PUSH TOKEN
  async updatePushToken(
    userId: string,
    pushToken: WebPushSubscription,
  ): Promise<{ push_token: WebPushSubscription | null; user_id: string }> {
    if (
      !pushToken ||
      !pushToken.endpoint ||
      !pushToken.keys?.p256dh ||
      !pushToken.keys?.auth
    ) {
      throw new BadRequestException('Invalid push token structure');
    }

    // Check uniqueness - Prisma doesn't support direct JSON equality in where clauses
    // So we serialize and use a raw query to check for duplicates
    const serializedToken = JSON.stringify(pushToken);
    const existing = await this.prisma.$queryRaw<Array<{ id: string }>>`
      SELECT id FROM "User" 
      WHERE push_token::text = ${serializedToken}::text
    `;

    if (existing.length > 0 && existing[0].id !== userId) {
      throw new ConflictException('Push token already in use');
    }

    // Update - cast WebPushSubscription to Prisma's InputJsonValue type
    const updated = await this.prisma.user.update({
      where: { id: userId },
      data: {
        push_token: pushToken as unknown as Prisma.InputJsonValue,
      },
      select: { id: true, push_token: true },
    });

    // Invalidate cache since user data changed
    await this.cacheService.invalidateUser(userId);

    return {
      user_id: updated.id,
      push_token: updated.push_token as WebPushSubscription | null,
    };
  }
}
