import { Injectable } from '@nestjs/common';
import { AuthGuard } from '@nestjs/passport';

// JwtAuthGaurd extends NestJS's built-in AuthGuard using the 'jwt' strategy to ensures that only requests with a valid JWT can proceed.

@Injectable()
export class JwtAuthGaurd extends AuthGuard('jwt') {
  // This guard automatically validates JWT tokens in incoming requests.
  // If the token is valid, the request proceeds; otherwise, it is denied.
}
