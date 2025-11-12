// src/middleware/proxy.module.ts
import { Module } from '@nestjs/common';
import { ProxyMiddleware } from './proxy.middleware';

@Module({
  providers: [ProxyMiddleware],
  exports: [ProxyMiddleware],
})
export class ProxyModule {}
