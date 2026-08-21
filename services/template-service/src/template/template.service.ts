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
  TemplateVersion,
} from '@prisma/client';
import {
  PaginatedResponse,
  PaginationMeta,
  RenderedMessage,
} from 'src/types/types';
import { CacheService } from '../common/cache.service';

@Injectable()
export class TemplateService {
  constructor(
    private prisma: PrismaService,
    private cacheService: CacheService,
  ) {
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

    const template = await this.prisma.$transaction(async (prisma) => {
      // Create template
      const newTemplate = await prisma.template.create({
        data: { name, event, channel, language },
        include: { versions: true },
      });

      // Create v1
      await prisma.templateVersion.create({
        data: { template_id: newTemplate.id, subject, title, body, variables },
      });

      return newTemplate;
    });

    // Invalidate cache for this event/channel/language combination
    for (const ch of channel) {
      await this.cacheService.invalidateTemplate(
        template.id,
        event,
        ch,
        language,
      );
    }

    return template;
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
    const cacheKey = `template:${id}:${includeHistory ? 'full' : 'latest'}`;

    // Try to get from cache first
    const cachedTemplate = await this.cacheService.get<
      Template & { versions: TemplateVersion[] }
    >(cacheKey);
    if (cachedTemplate) {
      return cachedTemplate;
    }

    // If not in cache, fetch from database
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

    // Cache for 30 minutes
    await this.cacheService.set(cacheKey, template, 1800);

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
      if (!latestVersion) {
        throw new NotFoundException('No version found for this template');
      }

      const newVersion = await this.prisma.templateVersion.create({
        data: {
          template_id: id,
          version: latestVersionNumber + 1,
          subject: updateDto.subject ?? latestVersion.subject ?? null,
          title: updateDto.title ?? latestVersion.title ?? null,
          body: updateDto.body ?? latestVersion.body,
          variables:
            (updateDto.variables as Prisma.InputJsonValue | undefined) ??
            (latestVersion.variables as Prisma.InputJsonValue | null) ??
            ({} as Prisma.InputJsonValue),
        },
      });

      await this.prisma.template.update({
        where: { id },
        data: { updated_at: new Date() },
      });

      // Invalidate cache
      await this.cacheService.invalidateTemplate(
        template.id,
        template.event,
        undefined,
        template.language,
      );

      return newVersion;
    }

    // Else, just update metadata
    const updated = await this.prisma.template.update({
      where: { id },
      data: updateDto,
    });

    // Invalidate cache
    await this.cacheService.invalidateTemplate(
      updated.id,
      updated.event,
      undefined,
      updated.language,
    );

    return updated;
  }

  //  Render with substitution
  /**
   * Compiles a template's latest version against a supplied context.
   *
   * Rendering is deliberately pure: it takes content and data and returns
   * content. Deciding *who* should receive a notification, and whether they
   * have consented, belongs to the orchestrator, which owns the user data.
   * This service only owns the message.
   */
  async render(
    templateId: string,
    dto?: RenderTemplateDto,
  ): Promise<RenderedMessage[]> {
    const context = dto?.data ?? {};

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
    const metadata = {
      templateId: template.id,
      templateVersion: latestVersion.version,
    };

    const results: RenderedMessage[] = [];

    for (const channel of template.channel) {
      if (channel === NotificationChannel.EMAIL) {
        results.push({
          channel,
          subject: this.compileTemplate(latestVersion.subject, context),
          html: this.compileTemplate(latestVersion.body, context),
          metadata,
        });
      }

      if (channel === NotificationChannel.PUSH) {
        results.push({
          channel,
          title: this.compileTemplate(latestVersion.title, context),
          body: this.compileTemplate(latestVersion.body, context),
          metadata,
        });
      }
    }

    if (!results.length)
      throw new NotFoundException('Template declares no renderable channels');

    return results;
  }

  private compileTemplate(
    templateString: string | null | undefined,
    context: Record<string, unknown>,
  ) {
    if (!templateString) return undefined;
    return Handlebars.compile(templateString)(context);
  }

  // Get by event/channel/lang (for dynamic sends)
  async getByEvent(
    event: string,
    channel: NotificationChannel,
    language = 'en',
  ) {
    const cacheKey = `template:event:${event}:${channel}:${language}`;

    // Try to get from cache first
    const cachedTemplate = await this.cacheService.get<
      Template & { versions: TemplateVersion[] }
    >(cacheKey);
    if (cachedTemplate) {
      return cachedTemplate;
    }

    // If not in cache, fetch from database
    const template = await this.prisma.template.findFirst({
      where: { event, channel: { has: channel }, language, isActive: true },
      include: { versions: { orderBy: { version: 'desc' }, take: 1 } },
    });
    if (!template)
      throw new NotFoundException(
        `No template for ${event}/${channel}/${language}`,
      );

    // Cache for 30 minutes
    await this.cacheService.set(cacheKey, template, 1800);

    return template;
  }

  //DELETE
  async delete(id: string) {
    // Get template first to invalidate cache properly
    const template = await this.prisma.template.findUnique({
      where: { id },
      select: { id: true, event: true, language: true, channel: true },
    });

    const deleted = await this.prisma.template.delete({
      where: { id },
    });

    // Invalidate cache
    if (template) {
      for (const ch of template.channel) {
        await this.cacheService.invalidateTemplate(
          template.id,
          template.event,
          ch,
          template.language,
        );
      }
    }

    return deleted;
  }
}
