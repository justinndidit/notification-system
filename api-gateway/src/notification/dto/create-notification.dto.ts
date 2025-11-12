import {
  IsString,
  IsNotEmpty,
  IsObject,
  IsOptional,
  IsEnum,
} from 'class-validator';

export enum NotificationChannel {
  EMAIL = 'EMAIL',
  PUSH = 'PUSH',
}

export class CreateNotificationDto {
  @IsString()
  @IsNotEmpty()
  userId: string;

  @IsString()
  @IsNotEmpty()
  event: string;

  @IsObject()
  @IsNotEmpty()
  data: Record<string, any>;

  @IsEnum(NotificationChannel, { each: true })
  @IsOptional()
  channels?: NotificationChannel[];

  @IsString()
  @IsOptional()
  language?: string;
}
