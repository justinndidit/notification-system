import {
  Body,
  Controller,
  Get,
  Param,
  Patch,
  Post,
  Query,
  Req,
  UnauthorizedException,
  UseGuards,
} from '@nestjs/common';

import {
  LoginDto,
  PaginationDto,
  RegisterDto,
  UpdatePreferenceDto,
} from './dto/user.dto';
import { UserService } from './user.service';
import { JwtAuthGaurd } from './jwt-auth.guard';

@Controller('user')
export class UserController {
  constructor(private userService: UserService) {}
  //SIGN UP
  @Post('/signup')
  signup(@Body() registerDto: RegisterDto) {
    return this.userService.signup(registerDto);
  }

  //SIGN IN
  @Post('/signin')
  signin(@Body() registerDto: LoginDto) {
    return this.userService.signin(registerDto);
  }

  //GET ALL USERS
  @Get('')
  @UseGuards(JwtAuthGaurd)
  getAllUsers(@Query() paginationDto: PaginationDto, @Req() req: JwtRequest) {
    const role = req.user.role;
    if (role !== 'admin') {
      throw new UnauthorizedException(
        'Forbidden: You are not authorized to perform this request',
      );
    }
    return this.userService.getPaginatedUsers(paginationDto);
  }

  //GET ALL PREFERENCE
  @Get('/preference')
  @UseGuards(JwtAuthGaurd)
  getAllUserPreference(
    @Query() paginationDto: PaginationDto,
    @Req() req: JwtRequest,
  ) {
    const role = req.user.role;
    if (role !== 'admin') {
      throw new UnauthorizedException(
        'Forbidden: You are not authorized to update this preference',
      );
    }
    return this.userService.getPaginatedUserPreferences(paginationDto);
  }

  //GET USER BY ID
  @Get('/:id')
  @UseGuards(JwtAuthGaurd)
  getUserById(@Param('id') userId: string) {
    return this.userService.getUserById(userId);
  }

  //GET USER PREFERENCE BY ID
  @Get('preference/:id')
  @UseGuards(JwtAuthGaurd)
  getUserPreference(@Param('id') userId: string) {
    return this.userService.getUserPreference(userId);
  }
  // UPDATE PREFERENCE
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

  //   update push token
  @Patch(':id/push-token')
  @UseGuards(JwtAuthGaurd)
  async updatePushToken(
    @Param('id') id: string,
    @Body('push_token') pushToken: WebPushSubscription,
    @Req() { user }: JwtRequest,
  ) {
    if (user.user_id !== id)
      throw new UnauthorizedException(
        'Forbidden, you are not allowed to update this push toke ',
      );
    return this.userService.updatePushToken(id, pushToken);
  }
}
