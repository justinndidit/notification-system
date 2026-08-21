import { Test, TestingModule } from '@nestjs/testing';
import { NotFoundException } from '@nestjs/common';
import { TemplateService } from './template.service';
import { PrismaService } from 'src/prisma/prisma.service';
import { CacheService } from '../common/cache.service';

const emailTemplate = {
  id: 'tpl-1',
  name: 'Welcome',
  event: 'USER_REGISTERED',
  channel: ['EMAIL'],
  language: 'en',
  isActive: true,
  versions: [
    {
      id: 'v2',
      version: 2,
      subject: 'Welcome, {{user.name}}',
      title: null,
      body: '<p>Hello {{user.name}}, visit <a href="{{link}}">us</a>.</p>',
    },
    {
      id: 'v1',
      version: 1,
      subject: 'Old subject',
      title: null,
      body: 'old body',
    },
  ],
};

describe('TemplateService', () => {
  let service: TemplateService;
  let prisma: { template: { findUnique: jest.Mock }; user: { findMany: jest.Mock } };

  beforeEach(async () => {
    prisma = {
      template: { findUnique: jest.fn() },
      // Present only to prove render never touches it.
      user: { findMany: jest.fn() },
    };

    const module: TestingModule = await Test.createTestingModule({
      providers: [
        TemplateService,
        { provide: PrismaService, useValue: prisma },
        {
          provide: CacheService,
          useValue: {
            get: jest.fn().mockResolvedValue(null),
            set: jest.fn().mockResolvedValue(undefined),
            delete: jest.fn().mockResolvedValue(undefined),
          },
        },
      ],
    }).compile();

    service = module.get<TemplateService>(TemplateService);
  });

  describe('render', () => {
    it('compiles the supplied context into the template', async () => {
      prisma.template.findUnique.mockResolvedValue(emailTemplate);

      const [message] = await service.render('tpl-1', {
        data: { user: { name: 'Ada' }, link: 'https://example.com' },
      });

      expect(message.subject).toBe('Welcome, Ada');
      expect(message.html).toContain('Hello Ada');
      expect(message.html).toContain('https://example.com');
    });

    it('renders the latest version, not the first', async () => {
      prisma.template.findUnique.mockResolvedValue(emailTemplate);

      const [message] = await service.render('tpl-1', {
        data: { user: { name: 'Ada' } },
      });

      // Templates are immutable and versioned; an in-flight notification must
      // render against the newest version, never a superseded one.
      expect(message.subject).toBe('Welcome, Ada');
      expect(message.metadata?.templateVersion).toBe(2);
    });

    it('does not look up users', async () => {
      prisma.template.findUnique.mockResolvedValue(emailTemplate);

      await service.render('tpl-1', { data: { user: { name: 'Ada' } } });

      // Rendering used to resolve recipients from this service's own User
      // table — a schema copy nothing writes to — so it returned 404 for every
      // input. Recipient selection belongs to the orchestrator.
      expect(prisma.user.findMany).not.toHaveBeenCalled();
    });

    it('renders with an empty context rather than failing', async () => {
      prisma.template.findUnique.mockResolvedValue(emailTemplate);

      const [message] = await service.render('tpl-1');

      expect(message.subject).toBe('Welcome, ');
    });

    it('emits one message per declared channel', async () => {
      prisma.template.findUnique.mockResolvedValue({
        ...emailTemplate,
        channel: ['EMAIL', 'PUSH'],
        versions: [
          {
            id: 'v1',
            version: 1,
            subject: 'Subject {{n}}',
            title: 'Title {{n}}',
            body: 'Body {{n}}',
          },
        ],
      });

      const messages = await service.render('tpl-1', { data: { n: '1' } });

      expect(messages).toHaveLength(2);

      const email = messages.find((m) => m.channel === 'EMAIL');
      const push = messages.find((m) => m.channel === 'PUSH');

      // Each channel carries its own fields: email has a subject and html,
      // push has a title and a plain body.
      expect(email?.subject).toBe('Subject 1');
      expect(email?.html).toBe('Body 1');
      expect(push?.title).toBe('Title 1');
      expect(push?.body).toBe('Body 1');
    });

    it('rejects an unknown template', async () => {
      prisma.template.findUnique.mockResolvedValue(null);

      await expect(service.render('missing')).rejects.toBeInstanceOf(
        NotFoundException,
      );
    });

    it('rejects a template with no versions', async () => {
      prisma.template.findUnique.mockResolvedValue({
        ...emailTemplate,
        versions: [],
      });

      await expect(service.render('tpl-1')).rejects.toBeInstanceOf(
        NotFoundException,
      );
    });
  });
});
