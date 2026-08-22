import { ExecutionContext } from '@nestjs/common';
import { Reflector } from '@nestjs/core';
import { JwtAuthGuard } from './jwt-auth.guard';
import { IS_PUBLIC_KEY } from './public.decorator';

/**
 * The gateway's auth previously decided whether a route was public by asking if
 * the URL *contained* "signin" or "signup". That made /template/signin-banner a
 * public route, and kept the same list in three places. These tests pin the
 * replacement: exemption comes from the handler's metadata, never the path.
 */
describe('JwtAuthGuard', () => {
  const contextFor = (url: string): ExecutionContext =>
    ({
      switchToHttp: () => ({ getRequest: () => ({ url, originalUrl: url }) }),
      getHandler: () => () => undefined,
      getClass: () => class {},
    }) as unknown as ExecutionContext;

  const guardWith = (isPublic: boolean | undefined) => {
    const reflector = {
      getAllAndOverride: jest.fn().mockReturnValue(isPublic),
    } as unknown as Reflector;

    const guard = new JwtAuthGuard(reflector);

    // Stand in for Passport's verification so these tests cover the guard's own
    // decision, not the strategy's.
    const superCanActivate = jest
      .spyOn(
        Object.getPrototypeOf(Object.getPrototypeOf(guard)) as {
          canActivate: () => boolean;
        },
        'canActivate',
      )
      .mockReturnValue(false);

    return { guard, reflector, superCanActivate };
  };

  afterEach(() => jest.restoreAllMocks());

  it('allows a handler marked @Public()', () => {
    const { guard } = guardWith(true);

    expect(guard.canActivate(contextFor('/health'))).toBe(true);
  });

  it('reads the exemption from handler metadata, not the URL', () => {
    const { guard, reflector } = guardWith(true);

    guard.canActivate(contextFor('/anything'));

    expect(reflector.getAllAndOverride).toHaveBeenCalledWith(
      IS_PUBLIC_KEY,
      expect.any(Array),
    );
  });

  it.each([
    ['/template/signin-banner'],
    ['/user/profile?q=signin'],
    ['/notifications/signup-reminder'],
  ])('does not treat %s as public just because of its path', (url) => {
    // Each of these was a genuine auth bypass under the old substring matching.
    const { guard, superCanActivate } = guardWith(undefined);

    guard.canActivate(contextFor(url));

    expect(superCanActivate).toHaveBeenCalled();
  });

  it.each([['/user'], ['/template'], ['/notifications'], ['/user/abc-123']])(
    'requires authentication for %s',
    (url) => {
      const { guard, superCanActivate } = guardWith(false);

      guard.canActivate(contextFor(url));

      expect(superCanActivate).toHaveBeenCalled();
    },
  );
});
