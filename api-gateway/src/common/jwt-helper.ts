import { Injectable, Logger } from '@nestjs/common';
import { ConfigService } from '@nestjs/config';
import * as jwt from 'jsonwebtoken';
import { Request as ExpressRequest } from 'express';

@Injectable()
export class JwtHelper {
  private readonly logger = new Logger(JwtHelper.name);

  constructor(private configService: ConfigService) {}

  validateToken(token: string): JwtPayload | null {
    try {
      const secret = this.configService.get<string>('JWT_SECRET');
      if (!secret) {
        this.logger.warn('JWT_SECRET is not configured');
        return null;
      }

      // Warn if using default placeholder secret
      if (
        secret === 'your-secret' ||
        secret === 'secret' ||
        secret.length < 10
      ) {
        this.logger.warn(
          'JWT_SECRET appears to be a placeholder. Please set a proper secret that matches your user-service.',
        );
      }

      const decoded = jwt.verify(token, secret) as JwtPayload;
      this.logger.debug(
        `Token validated successfully for user: ${decoded.user_id}`,
      );
      return decoded;
    } catch (error) {
      const errorMessage =
        error instanceof Error ? error.message : 'Unknown error';
      this.logger.warn(`Token validation failed: ${errorMessage}`);

      // Provide helpful error message for invalid signature
      if (errorMessage.includes('invalid signature')) {
        this.logger.error(
          'JWT_SECRET mismatch! The secret in API Gateway must match the secret used in user-service to sign tokens.',
        );
      }

      return null;
    }
  }

  extractTokenFromRequest(req: ExpressRequest): string | null {
    // Try multiple ways to get the authorization header
    const authHeader =
      (req.headers['authorization'] as string) ||
      (req.headers['Authorization'] as string) ||
      (req.headers['AUTHORIZATION'] as string) ||
      (req.get?.('authorization') as string) ||
      (req.get?.('Authorization') as string);

    if (!authHeader) {
      return null;
    }

    // Handle both 'Bearer token' and just 'token' formats
    if (typeof authHeader === 'string') {
      if (authHeader.startsWith('Bearer ')) {
        return authHeader.substring(7).trim();
      }
      // If it doesn't start with Bearer, assume it's just the token
      return authHeader.trim();
    }

    return null;
  }
}
