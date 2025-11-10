import {
  IsEmail,
  IsString,
  MinLength,
  IsOptional,
  IsBoolean,
  IsInt,
} from 'class-validator';

export class RegisterDto {
  @IsEmail({}, { message: 'Please provide a valid email address' })
  email: string;

  @IsString()
  @MinLength(6, { message: 'Password must be at least 6 characters long' })
  password: string;

  @IsOptional()
  @IsString()
  push_token?: string;

  @IsOptional()
  @IsString()
  role?: string;
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
