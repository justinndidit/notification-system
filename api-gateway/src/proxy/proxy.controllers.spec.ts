import 'reflect-metadata';
import { PATH_METADATA, METHOD_METADATA } from '@nestjs/common/constants';
import { RequestMethod } from '@nestjs/common';
import { UserProxyController } from './user-proxy.controller';
import { TemplateProxyController } from './template-proxy.controller';
import { NotificationProxyController } from './notification-proxy.controller';
import { IS_PUBLIC_KEY } from '../auth/public.decorator';

/**
 * These assert the routing surface rather than the proxying itself. The bugs
 * that actually happened here were structural: routes that bypassed guards, and
 * a wildcard that never matched a bare mount point.
 */
const isPublic = (target: object, method: string): boolean =>
  Reflect.getMetadata(IS_PUBLIC_KEY, (target as never)[method]) === true;

describe('proxy controllers', () => {
  describe('UserProxyController', () => {
    const proto = UserProxyController.prototype;

    it('exposes signup and signin as public', () => {
      expect(isPublic(proto, 'signup')).toBe(true);
      expect(isPublic(proto, 'signin')).toBe(true);
    });

    it('does not exempt anything else', () => {
      // The catch-all must stay guarded, or every authenticated user route is
      // open — which is precisely what the old raw-middleware proxy did.
      expect(isPublic(proto, 'authenticated')).toBe(false);
      expect(isPublic(proto, 'root')).toBe(false);
    });

    it('handles the bare mount point as well as sub-paths', () => {
      // A wildcard alone requires at least one segment, so GET /user fell
      // through to a 404 before the guard ever ran.
      expect(Reflect.getMetadata(PATH_METADATA, proto.root)).toBe('/');
      expect(Reflect.getMetadata(PATH_METADATA, proto.authenticated)).toBe(
        '*path',
      );
    });

    it('accepts every verb on the catch-all', () => {
      expect(Reflect.getMetadata(METHOD_METADATA, proto.authenticated)).toBe(
        RequestMethod.ALL,
      );
    });
  });

  describe('TemplateProxyController', () => {
    const proto = TemplateProxyController.prototype;

    it('exempts nothing', () => {
      expect(isPublic(proto, 'all')).toBe(false);
      expect(isPublic(proto, 'root')).toBe(false);
    });

    it('covers both the mount point and sub-paths', () => {
      expect(Reflect.getMetadata(PATH_METADATA, proto.root)).toBe('/');
      expect(Reflect.getMetadata(PATH_METADATA, proto.all)).toBe('*path');
    });
  });

  describe('NotificationProxyController', () => {
    const proto = NotificationProxyController.prototype;

    it('exempts nothing', () => {
      expect(isPublic(proto, 'all')).toBe(false);
      expect(isPublic(proto, 'root')).toBe(false);
    });

    it('covers both the mount point and sub-paths', () => {
      expect(Reflect.getMetadata(PATH_METADATA, proto.root)).toBe('/');
      expect(Reflect.getMetadata(PATH_METADATA, proto.all)).toBe('*path');
    });

    it('attaches correlation and idempotency headers, generating them if absent', () => {
      const controller = new NotificationProxyController({} as never);
      const headers = (
        controller as unknown as {
          headers: (req: unknown) => Record<string, string>;
        }
      ).headers({ headers: {} });

      expect(headers['X-Correlation-ID']).toMatch(/^[0-9a-f-]{36}$/);
      expect(headers['X-Idempotency-Key']).toMatch(/^[0-9a-f-]{36}$/);
    });

    it('preserves headers the client already supplied', () => {
      const controller = new NotificationProxyController({} as never);
      const headers = (
        controller as unknown as {
          headers: (req: unknown) => Record<string, string>;
        }
      ).headers({
        headers: {
          'x-correlation-id': 'caller-correlation',
          'x-idempotency-key': 'caller-key',
        },
      });

      // Replacing a caller's correlation id severs their ability to trace the
      // request; replacing their idempotency key breaks safe retries.
      expect(headers['X-Correlation-ID']).toBe('caller-correlation');
      expect(headers['X-Idempotency-Key']).toBe('caller-key');
    });
  });
});
