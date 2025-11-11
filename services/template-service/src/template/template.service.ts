import { ConflictException, Injectable } from '@nestjs/common';
import { PrismaService } from 'src/prisma/prisma.service';
import { CreateTemplateDto } from './dto/create.template.dto';

@Injectable()
export class TemplateService {
  constructor(private prisma: PrismaService) {}

  async create(createDto: CreateTemplateDto) {
    const { name, event, channel, language, subject, title, body, variables } =
      createDto;

    // Check for existing (unique constraint)
    const existing = await this.prisma.template.findUnique({
      where: {
        event_channel_language: {
          event: event,
          channel: channel,
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
      });

      // Create v1
      await prisma.templateVersion.create({
        data: { template_id: template.id, subject, title, body, variables },
      });

      return template;
    });
  }
}
