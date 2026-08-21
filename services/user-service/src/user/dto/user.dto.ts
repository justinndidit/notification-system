import { Type } from 'class-transformer';
import {
  IsArray,
  ValidateNested,
  IsEmail,
  IsString,
  MinLength,
  IsOptional,
  IsBoolean,
  IsInt,
  IsIn,
  Min,
  Max,
} from 'class-validator';

export class RegisterDto {
  @IsString()
  name: string;

  @IsString()
  @IsEmail({}, { message: 'Please provide a valid email address' })
  email: string;

  @IsString()
  @MinLength(6, { message: 'Password must be at least 6 characters long' })
  password: string;


  // `role` is intentionally absent. Accepting it here let any caller register
  // themselves as an admin. Roles are assigned via PATCH /user/:id/role.
}

export class UpdateRoleDto {
  @IsIn(['user', 'admin'], { message: 'Role must be either "user" or "admin"' })
  role: string;
}

export class LoginDto {
  @IsEmail({}, { message: 'Please provide a valid email address' })
  email: string;

  @IsString()
  @MinLength(6, { message: 'Password must be at least 6 characters long' })
  password: string;
}

export class UpdatePreferenceDto {
  @IsOptional()
  @IsBoolean()
  email_opt_in?: boolean;

  @IsOptional()
  @IsBoolean()
  push_opt_in?: boolean;

  @IsOptional()
  @IsInt()
  daily_limit?: number;

  @IsOptional()
  @IsString()
  language?: string;
}

export class PaginationDto {
  @IsOptional()
  @IsInt()
  @Min(1)
  page?: number = 1;

  @IsOptional()
  @IsInt()
  @Min(1)
  @Max(100) // Cap to prevent abuse
  limit?: number = 10;
}

export class DeviceTokenDto {
  @IsString()
  token: string;

  @IsIn(['android', 'ios'], { message: 'Platform must be "android" or "ios"' })
  platform: 'android' | 'ios';
}

export class UpdateDeviceTokensDto {
  @IsArray()
  @ValidateNested({ each: true })
  @Type(() => DeviceTokenDto)
  device_tokens: DeviceTokenDto[];
}
