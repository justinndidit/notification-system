declare interface JwtPayload {
  user_id: string;
  role: string;
}

declare interface JwtRequest extends Request {
  user: JwtPayload;
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

declare interface DeviceToken {
  token: string;
  platform: 'android' | 'ios';
}

declare interface DeliveryProfile {
  user_id: string;
  name: string;
  email: string;
  device_tokens: DeviceToken[];
  email_opt_in: boolean;
  push_opt_in: boolean;
  daily_limit: number;
  language: string;
}
