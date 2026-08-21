import { Module } from '@nestjs/common';
import { ProxyService } from './proxy.service';
import { UserProxyController } from './user-proxy.controller';
import { TemplateProxyController } from './template-proxy.controller';
import { NotificationProxyController } from './notification-proxy.controller';

@Module({
  controllers: [
    UserProxyController,
    TemplateProxyController,
    NotificationProxyController,
  ],
  providers: [ProxyService],
  exports: [ProxyService],
})
export class ProxyModule {}
