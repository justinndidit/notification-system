import {
  Controller,
  Post,
  Body,
  Req,
  UnauthorizedException,
  Get,
  Query,
  Patch,
  Param,
  Delete,
} from '@nestjs/common';
import { UseGuards } from '@nestjs/common';
import { TemplateService } from './template.service';
import { JwtAuthGuard } from '../auth/jwt-auth.guard';
import { ServiceOrJwtGuard } from '../common/service-or-jwt.guard';
import {
  CreateTemplateDto,
  PaginationDto,
  RenderTemplateDto,
  UpdateTemplateDto,
} from './dto/create.template.dto';
import type { JwtRequest, RenderedMessage } from 'src/types/types';
import { NotificationChannel } from '@prisma/client';

@Controller('template')
export class TemplateController {
  constructor(private readonly templatesService: TemplateService) {}
  @UseGuards(JwtAuthGuard)
  @Post()
  create(@Body() createTemplateDto: CreateTemplateDto, @Req() req: JwtRequest) {
    try {
      const role = req.user.role;
      if (role !== 'admin') {
        throw new UnauthorizedException(
          'Forbidden: You are not authorized to create a template',
        );
      }
      return this.templatesService.create(createTemplateDto);
    } catch (error) {
      console.log(error);
    }
  }

  @UseGuards(JwtAuthGuard)
  @Get()
  getAllTemplates(
    @Query() paginationDto: PaginationDto,
    @Query('name') name?: string,
    @Query('language') language?: string,
    @Query('event') event?: string,
    @Query('channel') channel?: string,
  ) {
    const channelEnum = channel
      ? [channel.toUpperCase() as NotificationChannel]
      : undefined;
    const filters = { name, language, event, channel: channelEnum };
    return this.templatesService.getPaginatedTemplates(paginationDto, filters);
  }

  @UseGuards(ServiceOrJwtGuard)
  @Get(':id')
  findOne(@Param('id') id: string, @Query('history') includeHistory?: boolean) {
    return this.templatesService.findOne(id, includeHistory);
  }

  @UseGuards(ServiceOrJwtGuard)
  @Post(':id/render')
  render(
    @Param('id') id: string,
    @Body() renderDto?: RenderTemplateDto,
  ): Promise<RenderedMessage[]> {
    return this.templatesService.render(id, renderDto);
  }

  @UseGuards(ServiceOrJwtGuard)
  @Get('event/:event/channel/:channel')
  getByEvent(
    @Param('event') event: string,
    @Param('channel') channel: NotificationChannel,
    @Query('language') language?: string,
  ) {
    const channelEnum = channel.toUpperCase() as NotificationChannel;
    return this.templatesService.getByEvent(
      event.toUpperCase(),
      channelEnum,
      language ?? 'en',
    );
  }

  @UseGuards(JwtAuthGuard)
  @Patch(':id')
  update(
    @Param('id') id: string,
    @Body() updateTemplateDto: UpdateTemplateDto,
  ) {
    return this.templatesService.update(id, updateTemplateDto);
  }

  @UseGuards(JwtAuthGuard)
  @Delete(':id')
  remove(@Param('id') id: string) {
    return this.templatesService.delete(id);
  }
}
