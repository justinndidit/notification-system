import {
  ConflictException,
  Injectable,
  NotFoundException,
} from '@nestjs/common';
import { PrismaService } from 'src/prisma/prisma.service';
import {
  CreateTemplateDto,
  PaginationDto,
  RenderTemplateDto,
  UpdateTemplateDto,
} from './dto/create.template.dto';
import * as Handlebars from 'handlebars';
import {
  NotificationChannel,
  Prisma,
  Template,
  Preference,
  User,
} from '@prisma/client';
import {
  PaginatedResponse,
  PaginationMeta,
  RenderedMessage,
} from 'src/types/types';

@Injectable()
export class TemplateService {
  constructor(private prisma: PrismaService) {
    // Register Handlebars helpers if needed (e.g., {{if}} for conditionals)
    Handlebars.registerHelper(
      'ifEquals',
      function (a: unknown, b: unknown, options: Handlebars.HelperOptions) {
        return a === b ? options.fn(this) : options.inverse(this);
      },
    );
  }

  //CREATE TEMPLATE
  async create(createDto: CreateTemplateDto) {
    const { name, event, channel, language, subject, title, body, variables } =
      createDto;

    // Check for existing (unique constraint)
    const existing = await this.prisma.template.findUnique({
      where: {
        event_language: {
          event: event,
          language: language,
        },
      }, // Composite unique
    });

    if (existing)
      throw new ConflictException(
        'Template exists for this event/channel/language',
      );

    return this.prisma.$transaction(async (prisma) => {
      // Create template
      const template = await prisma.template.create({
        data: { name, event, channel, language },
        include: { versions: true },
      });

      // Create v1
      await prisma.templateVersion.create({
        data: { template_id: template.id, subject, title, body, variables },
      });

      return template;
    });
  }

  //FIND TEMPLATE
  async getPaginatedTemplates(
    paginationDto: PaginationDto,
    filters?: {
      name?: string;
      language?: string;
      event?: string;
      channel?: NotificationChannel[];
    },
  ): Promise<PaginatedResponse<Template>> {
    const page = paginationDto.page ?? 1;
    const limit = paginationDto.limit ?? 10;
    const skip = (page - 1) * limit;

    // Build where clause (merge filters)
    const where = {
      isActive: true,
      ...(filters?.name && {
        name: { contains: filters.name, mode: Prisma.QueryMode.insensitive },
      }), // Fuzzy search
      ...(filters?.language && { language: filters.language }),
      ...(filters?.event && { event: filters.event }),
      ...(filters?.channel && { channel: { hasSome: filters.channel } }), // Array for multi-channel
    };

    // Parallel: count + data
    const [total, templates] = await Promise.all([
      this.prisma.template.count({ where }),
      this.prisma.template.findMany({
        where,
        skip,
        take: limit,
        orderBy: { created_at: 'desc' }, // Or { name: 'asc' }—sensible default
        include: { versions: { orderBy: { version: 'desc' }, take: 1 } }, // Latest version
      }),
    ]);
    const totalPages = Math.ceil(total / limit);
    const meta: PaginationMeta = {
      total,
      limit,
      page,
      total_pages: totalPages,
      has_next: page < totalPages,
      has_previous: page > 1,
    };

    if (!templates) {
      throw new NotFoundException(`No templates found on page ${page}`);
    }

    if (templates.length === 0 && page > 1) {
      throw new NotFoundException(`No templates found on page ${page}`);
    }

    return { data: templates, meta };
  }

  //FIND TEMPLATE BY ID
  async findOne(id: string, includeHistory = true) {
    const template = await this.prisma.template.findUnique({
      where: { id },
      include: {
        versions: {
          orderBy: { version: 'desc' },
          ...(includeHistory ? {} : { take: 1 }), // Take only latest if history not requested
        },
      },
    });
    if (!template) throw new NotFoundException('Template not found');
    return template;
  }

  //UPDATE TEMPLATE
  async update(id: string, updateDto: UpdateTemplateDto) {
    const template = await this.findOne(id);
    if (!template.isActive) throw new ConflictException('Inactive template');

    // If updating content, create new version
    if (
      updateDto.subject ||
      updateDto.title ||
      updateDto.body ||
      updateDto.variables
    ) {
      const latestVersionNumber = template.versions.reduce(
        (max, v) => (v.version > max ? v.version : max),
        0,
      );

      const latestVersion = template.versions[0];

      const newVersion = await this.prisma.templateVersion.create({
        data: {
          template_id: id,
          version: latestVersionNumber + 1,
          subject: updateDto.subject ?? latestVersion.subject,
          title: updateDto.title ?? latestVersion.title,
          body: updateDto.body ?? latestVersion.body,
          variables: updateDto.variables ?? latestVersion.variables ?? {},
        },
      });

      await this.prisma.template.update({
        where: { id },
        data: { updated_at: new Date() },
      });

      return newVersion;
    }

    // Else, just update metadata
    return this.prisma.template.update({ where: { id }, data: updateDto });
  }

  //  Render with substitution
  async render(
    templateId: string,
    dto?: RenderTemplateDto,
  ): Promise<RenderedMessage[]> {
    const payload = dto ?? {};
    const { data: dtoData, userId } = payload;
    const baseData: Record<string, unknown> = dtoData ?? {};

    const template = await this.prisma.template.findUnique({
      where: { id: templateId },
      include: {
        versions: {
          orderBy: { version: 'desc' },
        },
      },
    });

    if (!template) throw new NotFoundException('Template not found');
    if (!template.versions.length)
      throw new NotFoundException('No version available');

    const latestVersion = template.versions[0];
    const targetUsers = await this.resolveTargetUsers(template, userId);
    const results: RenderedMessage[] = [];

    for (const user of targetUsers) {
      const eligibleChannels = this.getEligibleChannelsForUser(
        user,
        template.channel,
      );

      if (!eligibleChannels.length) continue;

      const renderContext = this.buildRenderContext(user, baseData);

      for (const channel of eligibleChannels) {
        if (channel === NotificationChannel.EMAIL) {
          results.push({
            channel,
            subject: this.compileTemplate(latestVersion.subject, renderContext),
            html: this.compileTemplate(latestVersion.body, renderContext),
            recipient: this.mapRecipient(user),
            metadata: {
              templateId: template.id,
              templateVersion: latestVersion.version,
            },
          });
        }

        if (channel === NotificationChannel.PUSH) {
          results.push({
            channel,
            title: this.compileTemplate(latestVersion.title, renderContext),
            body: this.compileTemplate(latestVersion.body, renderContext),
            recipient: this.mapRecipient(user),
            metadata: {
              templateId: template.id,
              templateVersion: latestVersion.version,
            },
          });
        }
      }
    }

    if (!results.length)
      throw new NotFoundException(
        'No eligible recipients found for this template',
      );

    return results;
  }

  private async resolveTargetUsers(
    template: Template,
    userId?: string,
  ): Promise<Array<User & { preferences: Preference | null }>> {
    if (userId) {
      const user = await this.prisma.user.findUnique({
        where: { id: userId },
        include: { preferences: true },
      });

      if (!user) throw new NotFoundException('User not found');

      return [user];
    }

    return this.prisma.user.findMany({
      where: {
        preferences: {
          is: {
            language: template.language,
          },
        },
      },
      include: { preferences: true },
      orderBy: {
        created_at: 'asc',
      },
    });
  }

  private getEligibleChannelsForUser(
    user: User & { preferences: Preference | null },
    templateChannels: NotificationChannel[],
  ): NotificationChannel[] {
    const preferences = user.preferences;
    if (!preferences) return [];

    return templateChannels.filter((channel) => {
      if (channel === NotificationChannel.EMAIL) {
        return preferences.email_opt_in;
      }
      if (channel === NotificationChannel.PUSH) {
        return preferences.push_opt_in;
      }
      return false;
    });
  }

  private buildRenderContext(
    user: User & { preferences: Preference | null },
    data: Record<string, unknown>,
  ) {
    const { user: manualUserDataRaw, ...restData } = {
      ...data,
    } as Record<string, unknown> & { user?: unknown };
    const manualUserData =
      manualUserDataRaw && typeof manualUserDataRaw === 'object'
        ? (manualUserDataRaw as Record<string, unknown>)
        : undefined;
    return {
      ...restData,
      user: {
        id: user.id,
        name: user.name,
        email: user.email,
        role: user.role,
        push_token: user.push_token,
        preferences: user.preferences
          ? {
              email_opt_in: user.preferences.email_opt_in,
              push_opt_in: user.preferences.push_opt_in,
              language: user.preferences.language,
            }
          : undefined,
        ...(manualUserData ?? {}),
      },
    };
  }

  private compileTemplate(
    templateString: string | null | undefined,
    context: Record<string, unknown>,
  ) {
    if (!templateString) return undefined;
    return Handlebars.compile(templateString)(context);
  }

  private mapRecipient(user: User & { preferences: Preference | null }) {
    return {
      id: user.id,
      name: user.name,
      email: user.email,
      push_token: user.push_token ?? undefined,
    };
  }

  // Get by event/channel/lang (for dynamic sends)
  async getByEvent(
    event: string,
    channel: NotificationChannel,
    language = 'en',
  ) {
    const template = await this.prisma.template.findFirst({
      where: { event, channel: { has: channel }, language, isActive: true },
      include: { versions: { orderBy: { version: 'desc' }, take: 1 } },
    });
    if (!template)
      throw new NotFoundException(
        `No template for ${event}/${channel}/${language}`,
      );
    return template;
  }

  //DELETE
  async delete(id: string) {
    return this.prisma.template.delete({
      where: { id },
    });
  }
}
