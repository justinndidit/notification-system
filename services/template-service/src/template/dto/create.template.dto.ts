// src/templates/dto/create-template.dto.ts
import { PartialType } from '@nestjs/mapped-types';
import { NotificationChannel } from '@prisma/client';
import {
  IsEnum,
  IsString,
  IsOptional,
  IsBoolean,
  IsObject,
  IsInt,
  Min,
  Max,
} from 'class-validator';

export class CreateTemplateDto {
  @IsString()
  name: string;

  @IsString()
  event: string;

  @IsEnum(NotificationChannel, { each: true })
  channel: NotificationChannel[];

  @IsString()
  language: string;

  @IsOptional()
  @IsString()
  subject?: string; // EMAIL only

  @IsOptional()
  @IsString()
  title?: string; // PUSH only

  @IsString()
  body: string; // Handlebars template

  @IsObject()
  @IsOptional()
  variables?: Record<string, string>; // e.g., { "user.name": "string" }
}

export class UpdateTemplateDto extends PartialType(CreateTemplateDto) {
  @IsOptional()
  @IsBoolean()
  isActive?: boolean;
}

export class RenderTemplateDto {
  // Handlebars context, e.g. { user: { name: 'John' }, order: { id: '123' } }.
  // The caller supplies everything the template references; this service does
  // not look users up. Recipient selection belongs to the orchestrator.
  @IsOptional()
  @IsObject()
  data?: Record<string, unknown>;
}
export class PaginationDto {
  @IsOptional()
  @IsInt()
  @Min(1)
  page?: number = 1;

  @IsOptional()
  @IsInt()
  @Min(1)
  @Max(50) // Cap to prevent abuse
  limit?: number = 10;
}
