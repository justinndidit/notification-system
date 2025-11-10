/* eslint-disable @typescript-eslint/no-unused-vars */
import {
  ConflictException,
  Injectable,
  NotFoundException,
  UnauthorizedException,
} from '@nestjs/common';
import { JwtService } from '@nestjs/jwt';
import { PrismaService } from 'src/prisma/prisma.service';
import { LoginDto, RegisterDto, UpdatePreferenceDto } from './dto/user.dto';
import * as bcrypt from 'bcrypt';

@Injectable()
export class UserService {
  constructor(
    private prisma: PrismaService,
    private jwtService: JwtService,
  ) {}

  //SIGN UP
  async signup(registerDto: RegisterDto) {
    const { email, password, push_token, role } = registerDto;
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
  async getAllUsers() {
    const users = await this.prisma.user.findMany();
    if (!users) {
      throw new NotFoundException('Users not found');
    }

    const safeUsers = users.map(({ password: _, ...rest }) => rest);
    return safeUsers;
  }

  //PREFERENCE
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

    return { message: 'Preference updated successfully', updatedPreference };
  }

  //GET users preference by Ids
  async getUserPreference(userId: string) {
    const user = await this.prisma.user.findUnique({
      where: { id: userId },
      include: {
        preferences: true,
      },
    });

    if (!user) {
      throw new NotFoundException('User not found');
    }

    return user.preferences;
  }
}
