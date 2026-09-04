export const STORAGE_KEY = 'profile-blocks-template-v2';

export const initialProfile = {
  name: '你的名字',
  headline: '创作者 · 独立开发者',
  bio: '用作品和公开联系方式介绍你正在做的事。',
  tags: ['AI 产品', '构建', '创作'],
  avatar: '',
};

export const initialBlocks = [
  {
    id: 'work-example',
    type: 'work',
    title: '个人项目',
    description: '用一张作品卡讲清楚问题、过程和成果。',
    url: 'https://example.com',
    cover: '',
  },
  {
    id: 'contact-example',
    type: 'contact',
    platform: 'X / Twitter',
    name: '@yourhandle',
    url: 'https://x.com',
  },
];

export const blockDefinitions = [
  { type: 'work', label: '导入作品', description: '从链接或手动信息建立作品卡', icon: 'work' },
  { type: 'contact', label: '媒体与联系', description: '添加飞书、X、邮箱或 GitHub', icon: 'social' },
];

export const contactPlatforms = ['飞书', 'X / Twitter', '邮箱', 'GitHub'];
