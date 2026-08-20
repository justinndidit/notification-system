import { SetMetadata, CustomDecorator } from '@nestjs/common';

export const IS_PUBLIC_KEY = 'isPublic';

/**
 * Marks a route as exempt from the global JwtAuthGuard.
 *
 * Prefer this over adding paths to the guard's string list: it is attached to
 * the handler itself, so it can't drift out of sync with the route, and it
 * doesn't rely on substring matching against the URL.
 */
export const Public = (): CustomDecorator<string> =>
  SetMetadata(IS_PUBLIC_KEY, true);
