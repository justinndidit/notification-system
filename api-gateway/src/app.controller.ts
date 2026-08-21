import { Controller, Get } from '@nestjs/common';
import { SkipThrottle } from '@nestjs/throttler';
import { AppService } from './app.service';
import { Public } from './auth/public.decorator';

@Controller('health')
export class AppController {
  constructor(private readonly appService: AppService) {}

  // Exempt from rate limiting: container and load-balancer probes poll this
  // constantly, and a throttled health check flaps the service out of rotation
  // precisely when it is under load and most needs to stay in.
  @SkipThrottle()
  @Public()
  @Get()
  health(): string {
    return this.appService.health();
  }
}
