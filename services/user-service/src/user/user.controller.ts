import {
  Body,
  Controller,
  Get,
  Param,
  Patch,
  Post,
  Req,
  UnauthorizedException,
  UseGuards,
} from '@nestjs/common';

import { RegisterDto, UpdatePreferenceDto } from './dto/user.dto';
import { UserService } from './user.service';
import { JwtAuthGaurd } from './jwt-auth.guard';

@Controller('user')
export class UserController {
  constructor(private userService: UserService) {}

  @Post('/signup')
  signup(@Body() registerDto: RegisterDto) {
    return this.userService.signup(registerDto);
  }

  @Get('/signin')
  signin(@Body() registerDto: RegisterDto) {
    return this.userService.signin(registerDto);
  }

  @Get('')
  @UseGuards(JwtAuthGaurd)
  getAllUsers(@Req() req: JwtRequest) {
    const role = req.user.role;
    if (role !== 'admin') {
      throw new UnauthorizedException(
        'Forbidden: You are not authorized to perform this request',
      );
    }
    return this.userService.getAllUsers();
  }

  @Get('preference/:id')
  @UseGuards(JwtAuthGaurd)
  getUserPreference(@Param('id') userId: string, @Req() req: JwtRequest) {
    const role = req.user.role;
    if (role !== 'admin') {
      throw new UnauthorizedException(
        'Forbidden: You are not authorized to update this preference',
      );
    }
    return this.userService.getUserPreference(userId);
  }

  @Patch(':id/preference')
  @UseGuards(JwtAuthGaurd)
  updatePreference(
    @Param('id') userId: string,
    @Body() updateDto: UpdatePreferenceDto,
    @Req() req: JwtRequest,
  ) {
    const authUserId = req.user.user_id;
    if (authUserId !== userId) {
      throw new UnauthorizedException(
        'Forbidden: You are not authorized to update this preference',
      );
    }
    return this.userService.updatePreference(userId, updateDto);
  }
}
