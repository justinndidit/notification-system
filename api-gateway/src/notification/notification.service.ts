import {
  Injectable,
  Logger,
  BadRequestException,
  NotFoundException,
  Inject,
} from '@nestjs/common';
import { ConfigService } from '@nestjs/config';
import { HttpService } from '@nestjs/axios';
import Redis from 'ioredis';
import { v4 as uuidv4 } from 'uuid';
import {
  CreateNotificationDto,
  NotificationChannel,
} from './dto/create-notification.dto';

interface UserPreferences {
  email_opt_in: boolean;
  push_opt_in: boolean;
  daily_limit: number;
  language: string;
}

interface RenderedMessage {
  channel: 'EMAIL' | 'PUSH';
  subject?: string;
  html?: string;
  title?: string;
  body?: string;
}

export interface NotificationStatus {
  id: string;
  userId: string;
  event: string;
  channels: NotificationChannel[];
  status: 'pending' | 'queued' | 'processing' | 'sent' | 'failed';
  createdAt: string;
  updatedAt: string;
}

@Injectable()
export class NotificationService {
  private readonly logger = new Logger(NotificationService.name);
  private readonly userServiceUrl: string;
  private readonly templateServiceUrl: string;
  private readonly emailServiceUrl: string;
  private readonly pushServiceUrl: string;

  constructor(
    @Inject('REDIS_CLIENT') private readonly redis: Redis,
    private readonly httpService: HttpService,
    private readonly configService: ConfigService,
  ) {
    this.userServiceUrl =
      this.configService.get<string>('USER_SERVICE_URL') ||
      'http://localhost:3001';
    this.templateServiceUrl =
      this.configService.get<string>('TEMPLATE_SERVICE_URL') ||
      'http://localhost:3003';
    this.emailServiceUrl =
      this.configService.get<string>('EMAIL_SERVICE_URL') ||
      'http://localhost:3004';
    this.pushServiceUrl =
      this.configService.get<string>('PUSH_SERVICE_URL') ||
      'http://localhost:3005';
  }

  /**
   * Main method to process notification requests
   */
  async createNotification(
    dto: CreateNotificationDto,
    authToken: string,
  ): Promise<{
    notificationId: string;
    status: string;
    channels: NotificationChannel[];
  }> {
    const notificationId = uuidv4();
    const language = dto.language || 'en';

    try {
      // 1. Fetch user preferences
      const preferences = await this.fetchUserPreferences(
        dto.userId,
        authToken,
      );

      // 2. Determine which channels to use
      const channels = this.determineChannels(dto.channels, preferences);

      if (channels.length === 0) {
        throw new BadRequestException(
          'No valid notification channels available for this user',
        );
      }

      // 3. Fetch and render template for each channel
      const renderedMessages = await this.fetchAndRenderTemplate(
        dto.event,
        channels,
        language,
        dto.data,
        authToken,
      );

      // 4. Create notification status
      const notificationStatus: NotificationStatus = {
        id: notificationId,
        userId: dto.userId,
        event: dto.event,
        channels,
        status: 'pending',
        createdAt: new Date().toISOString(),
        updatedAt: new Date().toISOString(),
      };

      // 5. Store notification status in Redis
      await this.saveNotificationStatus(notificationId, notificationStatus);

      // 6. Route messages to appropriate queues
      await this.routeToQueues(notificationId, dto.userId, renderedMessages);

      // 7. Update status to queued
      notificationStatus.status = 'queued';
      notificationStatus.updatedAt = new Date().toISOString();
      await this.saveNotificationStatus(notificationId, notificationStatus);

      this.logger.log(
        `Notification ${notificationId} created and queued for user ${dto.userId} via channels: ${channels.join(', ')}`,
      );

      return {
        notificationId,
        status: 'queued',
        channels,
      };
    } catch (error: unknown) {
      const errorMessage =
        error instanceof Error ? error.message : 'Unknown error';
      const errorStack = error instanceof Error ? error.stack : undefined;
      this.logger.error(
        `Error creating notification: ${errorMessage}`,
        errorStack,
      );

      // Update status to failed
      const failedStatus: NotificationStatus = {
        id: notificationId,
        userId: dto.userId,
        event: dto.event,
        channels: dto.channels || [],
        status: 'failed',
        createdAt: new Date().toISOString(),
        updatedAt: new Date().toISOString(),
      };
      await this.saveNotificationStatus(notificationId, failedStatus);

      throw error;
    }
  }

  /**
   * Fetch user preferences from user-service
   */
  private async fetchUserPreferences(
    userId: string,
    authToken: string,
  ): Promise<UserPreferences> {
    try {
      const response = await this.httpService.axiosRef.get(
        `${this.userServiceUrl}/user/preference/${userId}`,
        {
          headers: {
            Authorization: `Bearer ${authToken}`,
            'Content-Type': 'application/json',
          },
        },
      );

      // Handle both wrapped and direct responses
      const responseData = response.data as
        | { success: boolean; data?: UserPreferences }
        | UserPreferences
        | null;

      let preferences: UserPreferences | null = null;
      if (
        responseData &&
        typeof responseData === 'object' &&
        'success' in responseData
      ) {
        preferences = responseData.data || null;
      } else if (responseData && typeof responseData === 'object') {
        preferences = responseData;
      }

      if (!preferences) {
        throw new NotFoundException('User preferences not found');
      }

      return {
        email_opt_in: preferences.email_opt_in ?? true,
        push_opt_in: preferences.push_opt_in ?? true,
        daily_limit: preferences.daily_limit ?? 100,
        language: preferences.language ?? 'en',
      };
    } catch (error: unknown) {
      const errorMessage =
        error instanceof Error ? error.message : 'Unknown error';
      this.logger.warn(
        `Error fetching user preferences: ${errorMessage}. Using defaults.`,
      );
      // Return default preferences if fetch fails
      return {
        email_opt_in: true,
        push_opt_in: true,
        daily_limit: 100,
        language: 'en',
      };
    }
  }

  /**
   * Determine which channels to use based on preferences and request
   */
  private determineChannels(
    requestedChannels: NotificationChannel[] | undefined,
    preferences: UserPreferences,
  ): NotificationChannel[] {
    const availableChannels: NotificationChannel[] = [];

    if (requestedChannels && requestedChannels.length > 0) {
      // Use requested channels if user has opted in
      for (const channel of requestedChannels) {
        if (channel === NotificationChannel.EMAIL && preferences.email_opt_in) {
          availableChannels.push(NotificationChannel.EMAIL);
        } else if (
          channel === NotificationChannel.PUSH &&
          preferences.push_opt_in
        ) {
          availableChannels.push(NotificationChannel.PUSH);
        }
      }
    } else {
      // Use all available channels based on preferences
      if (preferences.email_opt_in) {
        availableChannels.push(NotificationChannel.EMAIL);
      }
      if (preferences.push_opt_in) {
        availableChannels.push(NotificationChannel.PUSH);
      }
    }

    return availableChannels;
  }

  /**
   * Fetch template and render it for each channel
   */
  private async fetchAndRenderTemplate(
    event: string,
    channels: NotificationChannel[],
    language: string,
    data: Record<string, any>,
    authToken: string,
  ): Promise<RenderedMessage[]> {
    const renderedMessages: RenderedMessage[] = [];

    try {
      // For each channel, get the template and render it
      for (const channel of channels) {
        try {
          // Get template by event and channel
          const templateResponse = await this.httpService.axiosRef.get(
            `${this.templateServiceUrl}/template/event/${event}/channel/${channel}`,
            {
              params: { language },
              headers: {
                Authorization: `Bearer ${authToken}`,
                'Content-Type': 'application/json',
              },
            },
          );

          const templateData = templateResponse.data as
            | { success: boolean; data?: { id: string } }
            | { id: string }
            | null;

          const template =
            templateData &&
            typeof templateData === 'object' &&
            'success' in templateData
              ? templateData.data || null
              : (templateData as { id: string } | null);

          if (!template || !template.id) {
            this.logger.warn(
              `Template not found for event: ${event}, channel: ${channel}`,
            );
            continue;
          }

          // Render the template
          const renderResponse = await this.httpService.axiosRef.post(
            `${this.templateServiceUrl}/template/${template.id}/render`,
            { data },
            {
              headers: {
                Authorization: `Bearer ${authToken}`,
                'Content-Type': 'application/json',
              },
            },
          );

          if (renderResponse.data) {
            // Template service returns RenderedMessage[] directly or wrapped
            const responseData = renderResponse.data as
              | RenderedMessage[]
              | { data?: RenderedMessage[] }
              | RenderedMessage
              | null;

            let rendered: RenderedMessage[] = [];

            if (Array.isArray(responseData)) {
              rendered = responseData;
            } else if (
              responseData &&
              typeof responseData === 'object' &&
              'data' in responseData &&
              Array.isArray(responseData.data)
            ) {
              rendered = responseData.data;
            } else if (
              responseData &&
              typeof responseData === 'object' &&
              'channel' in responseData
            ) {
              rendered = [responseData];
            }

            // Filter rendered messages for the current channel
            const channelMessages = rendered.filter(
              (msg) => msg.channel === (channel as 'EMAIL' | 'PUSH'),
            );
            renderedMessages.push(...channelMessages);
          }
        } catch (channelError: unknown) {
          const errorMessage =
            channelError instanceof Error
              ? channelError.message
              : 'Unknown error';
          this.logger.warn(
            `Error processing channel ${channel} for event ${event}: ${errorMessage}`,
          );
          // Continue with other channels even if one fails
          continue;
        }
      }
    } catch (error: unknown) {
      const errorMessage =
        error instanceof Error ? error.message : 'Unknown error';
      this.logger.error(`Error fetching/rendering template: ${errorMessage}`);
      throw new BadRequestException(
        `Failed to render template: ${errorMessage}`,
      );
    }

    if (renderedMessages.length === 0) {
      throw new NotFoundException(
        'No templates found for the specified event and channels',
      );
    }

    return renderedMessages;
  }

  /**
   * Route messages to appropriate Redis queues
   */
  private async routeToQueues(
    notificationId: string,
    userId: string,
    messages: RenderedMessage[],
  ): Promise<void> {
    for (const message of messages) {
      const queueMessage = {
        notificationId,
        userId,
        channel: message.channel,
        content: message,
        timestamp: new Date().toISOString(),
      };

      if (message.channel === 'EMAIL') {
        await this.redis.lpush('email:queue', JSON.stringify(queueMessage));
        this.logger.log(
          `Queued email notification ${notificationId} for user ${userId}`,
        );
      } else if (message.channel === 'PUSH') {
        await this.redis.lpush('push:queue', JSON.stringify(queueMessage));
        this.logger.log(
          `Queued push notification ${notificationId} for user ${userId}`,
        );
      }
    }
  }

  /**
   * Save notification status to Redis
   */
  private async saveNotificationStatus(
    notificationId: string,
    status: NotificationStatus,
  ): Promise<void> {
    const key = `notification:status:${notificationId}`;
    await this.redis.setex(key, 86400 * 7, JSON.stringify(status)); // 7 days TTL
  }

  /**
   * Get notification status
   */
  async getNotificationStatus(
    notificationId: string,
  ): Promise<NotificationStatus | null> {
    const key = `notification:status:${notificationId}`;
    const data = await this.redis.get(key);

    if (!data) {
      return null;
    }

    return JSON.parse(data) as NotificationStatus;
  }

  /**
   * Update notification status
   */
  async updateNotificationStatus(
    notificationId: string,
    status: 'pending' | 'queued' | 'processing' | 'sent' | 'failed',
  ): Promise<void> {
    const currentStatus = await this.getNotificationStatus(notificationId);

    if (!currentStatus) {
      throw new NotFoundException(`Notification ${notificationId} not found`);
    }

    currentStatus.status = status;
    currentStatus.updatedAt = new Date().toISOString();

    await this.saveNotificationStatus(notificationId, currentStatus);
  }
}
