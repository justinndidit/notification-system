// src/templates/types/template.types.ts

import { NotificationChannel, TemplateEvent } from '@prisma/client';

declare interface TemplateVariables {
  [key: string]: string;
}

declare interface CreateTemplateType {
  name: string;
  event: TemplateEvent;
  channel: NotificationChannel;
  language: string;
  subject?: string;
  title?: string;
  body: string;
  variables?: TemplateVariables;
}

declare interface UpdateTemplateType extends Partial<CreateTemplateType> {
  isActive?: boolean;
}

declare interface RenderTemplateType {
  templateId: string;
  data: Record<string, any>;
}

declare interface JwtPayload {
  user_id: string;
  role: string;
}

declare interface JwtRequest extends Request {
  user: JwtPayload;
}

declare interface RenderedMessage {
  channel: NotificationChannel;
  subject?: string;
  html?: string;
  title?: string;
  body?: string;
}

declare interface PaginationMeta {
  total: number;
  limit: number;
  page: number;
  total_pages: number;
  has_next: boolean;
  has_previous: boolean;
}

declare interface PaginatedResponse<T> {
  data: T[];
  meta: PaginationMeta;
}
