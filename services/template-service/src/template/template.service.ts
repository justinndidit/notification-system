import {
  ConflictException,
  Injectable,
  NotFoundException,
} from '@nestjs/common';
import { PrismaService } from 'src/prisma/prisma.service';
import {
  CreateTemplateDto,
  RenderTemplateDto,
  UpdateTemplateDto,
} from './dto/create.template.dto';
import * as Handlebars from 'handlebars';
import { NotificationChannel } from '@prisma/client';
import { RenderedMessage } from 'src/types/types';

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
  async findAll(
    name?: string,
    language = 'en',
    event?: string,
    channel?: NotificationChannel,
  ) {
    return this.prisma.template.findMany({
      where: {
        ...(name && { name }),
        ...(language && { language }),
        ...(event && { event }),
        ...(channel && { channel: { has: channel } }),
        isActive: true,
      },
      include: { versions: { orderBy: { version: 'desc' }, take: 1 } }, // Latest version
    });
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
    dto: RenderTemplateDto,
  ): Promise<RenderedMessage[]> {
    const { data } = dto;

    // Fetch template including versions
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
    const results: RenderedMessage[] = [];

    for (const channel of template.channel) {
      if (channel === 'EMAIL') {
        results.push({
          channel: 'EMAIL',
          subject: Handlebars.compile(latestVersion.subject || '')(data),
          html: Handlebars.compile(latestVersion.body || '')(data),
        });
      } else if (channel === 'PUSH') {
        results.push({
          channel: 'PUSH',
          title: Handlebars.compile(latestVersion.title || '')(data),
          body: Handlebars.compile(latestVersion.body || '')(data),
        });
      }
    }

    return results;
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
