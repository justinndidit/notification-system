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
  UpdateRoleDto,
  UpdateDeviceTokensDto,
} from './dto/user.dto';
import { UserService } from './user.service';
import { JwtAuthGaurd } from './jwt-auth.guard';
import { ServiceOrJwtGuard } from '../common/service-or-jwt.guard';

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
  // Called by end users and by the orchestrator during enrichment, so it
  // accepts either a user JWT or the internal service token.
  @Get('preference/:id')
  @UseGuards(ServiceOrJwtGuard)
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

  //GET DELIVERY PROFILE
  // Single call used by the orchestrator during enrichment: contact details
  // plus consent flags, so it doesn't have to stitch several endpoints together.
  @Get(':id/delivery-profile')
  @UseGuards(ServiceOrJwtGuard)
  getDeliveryProfile(@Param('id') userId: string) {
    return this.userService.getDeliveryProfile(userId);
  }

  // UPDATE ROLE (admin only)
  // Role is deliberately not settable at signup — it is assigned here by an
  // existing admin, so a self-service registration cannot escalate itself.
  @Patch(':id/role')
  @UseGuards(JwtAuthGaurd)
  updateRole(
    @Param('id') userId: string,
    @Body() updateRoleDto: UpdateRoleDto,
    @Req() req: JwtRequest,
  ) {
    if (req.user.role !== 'admin') {
      throw new UnauthorizedException(
        'Forbidden: You are not authorized to change user roles',
      );
    }
    return this.userService.updateRole(userId, updateRoleDto.role);
  }

  //   update device tokens
  @Patch(':id/device-tokens')
  @UseGuards(JwtAuthGaurd)
  async updateDeviceTokens(
    @Param('id') id: string,
    @Body() dto: UpdateDeviceTokensDto,
    @Req() { user }: JwtRequest,
  ) {
    if (user.user_id !== id)
      throw new UnauthorizedException(
        'Forbidden: you are not allowed to update this user\'s device tokens',
      );
    return this.userService.updateDeviceTokens(id, dto.device_tokens);
  }
}
