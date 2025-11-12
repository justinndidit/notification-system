import {
  Injectable,
  NestInterceptor,
  ExecutionContext,
  CallHandler,
} from '@nestjs/common';
import { Observable } from 'rxjs';
import { tap } from 'rxjs/operators';
import { Logger } from '@nestjs/common'; // Built-in

@Injectable()
export class LoggingInterceptor implements NestInterceptor {
  private readonly logger = new Logger(LoggingInterceptor.name);

  intercept(context: ExecutionContext, next: CallHandler): Observable<any> {
    const req = context
      .switchToHttp()
      .getRequest<{ method?: string; url?: string; body?: unknown }>();
    const method = req?.method ?? 'UNKNOWN_METHOD';
    const url = req?.url ?? 'UNKNOWN_URL';
    const body = req?.body ?? {};

    this.logger.log(`Received: ${method} ${url}`);
    this.logger.log(`Request body: ${JSON.stringify(body)}`);

    return next.handle().pipe(
      tap(() => {
        const res = context
          .switchToHttp()
          .getResponse<{ statusCode?: number }>();
        this.logger.log(
          `Response: ${method} ${url} - Status: ${res?.statusCode ?? 'UNKNOWN'}`,
        );
      }),
    );
  }
}
