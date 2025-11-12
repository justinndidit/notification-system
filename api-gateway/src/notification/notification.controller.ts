import {
  Controller,
  Post,
  Get,
  Body,
  Param,
  UseGuards,
  Req,
  HttpCode,
  HttpStatus,
  BadRequestException,
} from '@nestjs/common';
import {
  NotificationService,
  NotificationStatus,
} from './notification.service';
import { CreateNotificationDto } from './dto/create-notification.dto';
import { JwtAuthGuard } from '../auth/jwt-auth.guard';
import { Request } from 'express';

interface JwtRequest extends Request {
  user?: {
    user_id: string;
    role: string;
  };
}

@Controller('notifications')
@UseGuards(JwtAuthGuard)
export class NotificationController {
  constructor(private readonly notificationService: NotificationService) {}

  @Post()
  @HttpCode(HttpStatus.ACCEPTED)
  async createNotification(
    @Body() createNotificationDto: CreateNotificationDto,
    @Req() req: JwtRequest,
  ) {
    // Extract auth token from request
    const authToken = req.headers.authorization?.replace('Bearer ', '') || '';

    // Use userId from JWT if not provided in body
    const userId = createNotificationDto.userId || req.user?.user_id;

    if (!userId) {
      throw new BadRequestException('User ID is required');
    }

    const dto = {
      ...createNotificationDto,
      userId,
    };

    const result = await this.notificationService.createNotification(
      dto,
      authToken,
    );

    return {
      success: true,
      message: 'Notification queued successfully',
      data: result,
    };
  }

  @Get(':id/status')
  async getNotificationStatus(@Param('id') id: string): Promise<{
    success: boolean;
    message: string;
    data: NotificationStatus | null;
  }> {
    const status = await this.notificationService.getNotificationStatus(id);

    if (!status) {
      return {
        success: false,
        message: 'Notification not found',
        data: null,
      };
    }

    return {
      success: true,
      message: 'Notification status retrieved successfully',
      data: status,
    };
  }
}
