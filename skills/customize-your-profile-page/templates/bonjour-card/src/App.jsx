import React, { useEffect, useMemo, useRef, useState } from 'react';
import { blockDefinitions, contactPlatforms, initialBlocks, initialProfile, STORAGE_KEY } from './data';

const blankBlock = (type, platform = '') => ({
  id: crypto.randomUUID(),
  type,
  title: '',
  description: '',
  url: '',
  cover: '',
  platform,
  name: '',
  isPlaceholder: type === 'work',
});

function loadInitialState() {
  try {
    const stored = localStorage.getItem(STORAGE_KEY);
    if (!stored) return { profile: initialProfile, blocks: initialBlocks };
    const parsed = JSON.parse(stored);
    return {
      profile: { ...initialProfile, ...parsed.profile },
      blocks: Array.isArray(parsed.blocks) ? parsed.blocks : initialBlocks,
    };
  } catch {
    return { profile: initialProfile, blocks: initialBlocks };
  }
}

function saveState(profile, blocks) {
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify({ profile, blocks }));
  } catch {
    // ViceMe serves published pages in an opaque sandbox where storage is unavailable.
  }
}

function applyPlatformContext(context, setProfile, setBlocks) {
  if (context?.type !== 'CREATOR' || !context.creator) return;
  setProfile((current) => ({
    ...current,
    name: context.creator.displayName || current.name,
    headline: context.creator.occupation || current.headline,
    bio: context.creator.bio || current.bio,
    avatar: context.creator.avatarUrl || current.avatar,
  }));
  const importedWorks = Array.isArray(context.works) ? context.works.map((work) => ({
    id: `viceme-work-${work.slug}`,
    type: 'work',
    title: work.title,
    description: work.summary,
    url: work.canonicalPath,
    cover: work.coverUrl || '',
    workSlug: work.slug,
  })) : [];
  const importedContacts = Array.isArray(context.creator.externalIdentities)
    ? context.creator.externalIdentities
      .filter((identity) => identity.provider === 'GITHUB' && identity.profileUrl)
      .map((identity) => ({
        id: 'viceme-contact-github',
        type: 'contact',
        platform: 'GitHub',
        name: identity.externalHandle ? `@${identity.externalHandle.replace(/^@/, '')}` : 'GitHub',
        url: identity.profileUrl,
      }))
    : [];
  if (importedWorks.length === 0 && importedContacts.length === 0) return;
  setBlocks((current) => {
    const importedSlugs = new Set(importedWorks.map((block) => block.workSlug));
    const importedContactIds = new Set(importedContacts.map((block) => block.id));
    const authored = current.filter((block) => {
      if (importedWorks.length > 0 && block.id === 'work-example' && block.url === 'https://example.com') return false;
      if (importedContacts.length > 0 && block.id === 'contact-example' && block.url === 'https://x.com') return false;
      return !importedSlugs.has(block.workSlug) && !importedContactIds.has(block.id);
    });
    return [...importedWorks, ...importedContacts, ...authored];
  });
}

function Icon({ name, size = 20 }) {
  const common = { width: size, height: size, viewBox: '0 0 24 24', fill: 'none', stroke: 'currentColor', strokeWidth: 1.8, strokeLinecap: 'round', strokeLinejoin: 'round', 'aria-hidden': true };
  if (name === 'plus') return <svg {...common}><path d="M12 5v14M5 12h14" /></svg>;
  if (name === 'work') return <svg {...common}><rect x="4" y="4" width="16" height="16" rx="3" /><path d="M8 10h8M8 14h5" /></svg>;
  if (name === 'social') return <svg {...common}><path d="M10.4 13.6a3 3 0 0 0 4.2 0l2.3-2.3a3 3 0 1 0-4.2-4.2l-1.3 1.3" /><path d="M13.6 10.4a3 3 0 0 0-4.2 0l-2.3 2.3a3 3 0 1 0 4.2 4.2l1.3-1.3" /></svg>;
  if (name === 'edit') return <svg {...common}><path d="M4 20h4l10.5-10.5a2.1 2.1 0 0 0-4-2L4 18v2Z" /><path d="m13.5 8.5 2 2" /></svg>;
  if (name === 'trash') return <svg {...common}><path d="M4 7h16M10 11v5M14 11v5M6 7l1 13h10l1-13M9 7V4h6v3" /></svg>;
  if (name === 'close') return <svg {...common}><path d="m6 6 12 12M18 6 6 18" /></svg>;
  if (name === 'external') return <svg {...common}><path d="M14 4h6v6M20 4l-9 9" /><path d="M18 13v5a2 2 0 0 1-2 2H6a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h5" /></svg>;
  return null;
}

function readFile(file) {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => resolve(reader.result);
    reader.onerror = reject;
    reader.readAsDataURL(file);
  });
}

function displayNameFromUrl(url, platform) {
  if (platform === '邮箱') return url.replace(/^mailto:/, '');
  try {
    const parsed = new URL(url);
    const parts = parsed.pathname.split('/').filter(Boolean);
    if (platform === 'X / Twitter' && parts[0]) return `@${parts[0].replace('@', '')}`;
    if (platform === 'GitHub' && parts[0]) return `@${parts[0].replace('@', '')}`;
    if (platform === '飞书') return '在飞书联系我';
  } catch {
    return '';
  }
  return '';
}

function App() {
  const initial = useMemo(loadInitialState, []);
  const [profile, setProfile] = useState(initial.profile);
  const [blocks, setBlocks] = useState(initial.blocks);
  const [pickerOpen, setPickerOpen] = useState(false);
  const [modal, setModal] = useState(null);
  const [notice, setNotice] = useState('');
  const pickerRef = useRef(null);

  useEffect(() => {
    saveState(profile, blocks);
  }, [profile, blocks]);

  useEffect(() => {
    let active = true;
    if (typeof window.viceme?.context?.get !== 'function') return undefined;
    window.viceme.context.get()
      .then((context) => {
        if (active) applyPlatformContext(context, setProfile, setBlocks);
      })
      .catch(() => {});
    return () => { active = false; };
  }, []);

  useEffect(() => {
    const closePicker = (event) => {
      if (pickerRef.current && !pickerRef.current.contains(event.target)) setPickerOpen(false);
    };
    document.addEventListener('pointerdown', closePicker);
    return () => document.removeEventListener('pointerdown', closePicker);
  }, []);

  const addBlock = (definition) => {
    if (definition.type === 'contact') {
      setModal({ kind: 'contact-choice' });
    } else if (definition.type === 'work') {
      const block = blankBlock('work');
      setBlocks((current) => [...current, block]);
      setNotice('已创建作品占位卡，点击卡片即可补充信息。');
    } else {
      setModal({ kind: 'block', block: blankBlock(definition.type), isNew: true });
    }
    setPickerOpen(false);
  };

  const openEditor = (block) => setModal({ kind: 'block', block, isNew: false });
  const saveBlock = (nextBlock) => {
    const finalized = { ...nextBlock, isPlaceholder: false };
    setBlocks((current) => current.some((block) => block.id === finalized.id)
      ? current.map((block) => block.id === finalized.id ? finalized : block)
      : [...current, finalized]);
    setModal(null);
    setNotice('Block 已保存。');
  };
  const deleteBlock = (id) => {
    setBlocks((current) => current.filter((block) => block.id !== id));
    setModal(null);
    setNotice('Block 已删除。');
  };
  return (
    <div className="app-shell">
      <header className="topbar">
        <a className="brand" href="#profile" aria-label="回到编辑器顶部"><span className="brand-mark"><span /></span> Profile Blocks</a>
      </header>

      <main className="workspace" id="profile">
        <ProfileRail profile={profile} onEdit={() => setModal({ kind: 'profile' })} />
        <section className="canvas" aria-labelledby="canvas-title">
          <div className="canvas-heading">
            <div><h1 id="canvas-title">内容 Block</h1><p className="canvas-copy">作品与媒体联系方式会在这里组织成你的公开主页。</p></div>
            <div className="add-control" ref={pickerRef}>
              <button className="primary-button" type="button" aria-expanded={pickerOpen} onClick={() => setPickerOpen((open) => !open)}><Icon name="plus" />添加 Block</button>
              {pickerOpen && <BlockPicker onSelect={addBlock} />}
            </div>
          </div>

          {notice && <p className="notice" role="status">{notice}<button type="button" onClick={() => setNotice('')} aria-label="关闭提示">×</button></p>}
          <div className="block-stack">
            {blocks.map((block) => <BlockCard key={block.id} block={block} onEdit={() => openEditor(block)} onOpen={() => setModal({ kind: 'link-preview', block })} />)}
          </div>
          {blocks.length === 0 && <button className="empty-state" type="button" onClick={() => setPickerOpen(true)}><Icon name="plus" size={22} /><span>还没有内容 Block</span><small>从作品或联系方式开始。</small></button>}
        </section>
      </main>

      {modal?.kind === 'profile' && <ProfileModal profile={profile} onSave={(next) => { setProfile(next); setModal(null); setNotice('个人资料已保存。'); }} onClose={() => setModal(null)} />}
      {modal?.kind === 'contact-choice' && <ContactChoiceModal onChoose={(platform) => setModal({ kind: 'block', block: blankBlock('contact', platform), isNew: true })} onClose={() => setModal(null)} />}
      {modal?.kind === 'block' && <BlockModal block={modal.block} onSave={saveBlock} onDelete={() => deleteBlock(modal.block.id)} onClose={() => setModal(null)} />}
      {modal?.kind === 'link-preview' && <LinkPreviewModal block={modal.block} onClose={() => setModal(null)} />}
    </div>
  );
}

function ProfileRail({ profile, onEdit }) {
  return <aside className="profile-rail" aria-label="个人资料预览">
    <div className="profile-rail-head"><button className="icon-button" type="button" onClick={onEdit} aria-label="编辑个人资料"><Icon name="edit" size={18} /></button></div>
    <div className={`avatar ${profile.avatar ? 'has-image' : ''}`} style={profile.avatar ? { backgroundImage: `url(${profile.avatar})` } : undefined} aria-label={profile.avatar ? '个人头像' : '头像占位'} />
    <h2>{profile.name}</h2><p className="headline">{profile.headline}</p><p className="bio">{profile.bio}</p>
    <div className="rail-rule" />
    <p className="section-label">主题标签</p><div className="tag-list">{profile.tags.filter(Boolean).map((tag) => <span key={tag}>{tag}</span>)}</div>
  </aside>;
}

function BlockPicker({ onSelect }) {
  return <div className="block-picker" role="menu" aria-label="选择 Block 类型">
    <p>添加到主页</p>
    <div>{blockDefinitions.map((definition) => <button key={definition.type} type="button" role="menuitem" onClick={() => onSelect(definition)}><span className="picker-icon"><Icon name={definition.icon} size={20} /></span><span><strong>{definition.label}</strong><small>{definition.description}</small></span></button>)}</div>
  </div>;
}

function BlockCard({ block, onEdit, onOpen }) {
  const definition = blockDefinitions.find((item) => item.type === block.type) ?? blockDefinitions[1];
  const title = block.type === 'contact' ? block.name || `${block.platform} 主页` : block.title || (block.isPlaceholder ? '补充作品信息' : definition.label);
  const description = block.type === 'contact' ? block.url || '填写公开联系方式' : block.description || (block.isPlaceholder ? '点击卡片，添加链接、封面与作品简介。' : '点击补充信息');
  const canOpen = Boolean(block.url && !block.isPlaceholder);
  return <article className={`block-card block-${block.type} ${block.isPlaceholder ? 'is-placeholder' : ''}`}>
    {block.type === 'work' && block.cover ? <div className="block-cover" style={{ backgroundImage: `url(${block.cover})` }} aria-hidden="true" /> : <div className="block-symbol"><Icon name={definition.icon} size={21} /></div>}
    <button className="block-main" type="button" onClick={canOpen ? onOpen : onEdit} aria-label={canOpen ? `查看 ${title} 的链接` : `编辑 ${title}`}><span className="block-type">{block.type === 'contact' ? block.platform : definition.label}</span><strong>{title}</strong><span>{description}</span></button>
    <button className="icon-button block-edit" type="button" onClick={onEdit} aria-label={`编辑 ${title}`}><Icon name="edit" size={18} /></button>
  </article>;
}

function Dialog({ title, children, onClose }) {
  const dialogRef = useRef(null);
  const lastFocused = useRef(document.activeElement);

  useEffect(() => {
    const dialog = dialogRef.current;
    const focusableSelector = 'button:not([disabled]), input:not([disabled]), textarea:not([disabled])';
    const focusable = () => [...dialog.querySelectorAll(focusableSelector)];
    const first = focusable()[0];
    first?.focus();

    const handleKeyDown = (event) => {
      if (event.key === 'Escape') onClose();
      if (event.key !== 'Tab') return;
      const controls = focusable();
      const firstControl = controls[0];
      const lastControl = controls.at(-1);
      if (!firstControl || !lastControl) return;
      if (event.shiftKey && document.activeElement === firstControl) {
        event.preventDefault();
        lastControl.focus();
      } else if (!event.shiftKey && document.activeElement === lastControl) {
        event.preventDefault();
        firstControl.focus();
      }
    };
    dialog.addEventListener('keydown', handleKeyDown);
    return () => {
      dialog.removeEventListener('keydown', handleKeyDown);
      lastFocused.current?.focus();
    };
  }, []);

  return <div className="dialog-backdrop" role="presentation" onMouseDown={(event) => { if (event.target === event.currentTarget) onClose(); }}><section className="dialog" ref={dialogRef} role="dialog" aria-modal="true" aria-labelledby="dialog-title"><header><h2 id="dialog-title">{title}</h2><button className="icon-button" type="button" onClick={onClose} aria-label="关闭"><Icon name="close" /></button></header>{children}</section></div>;
}

function ProfileModal({ profile, onSave, onClose }) {
  const [draft, setDraft] = useState(profile);
  const [error, setError] = useState('');
  const uploadAvatar = async (event) => {
    const file = event.target.files?.[0];
    if (!file) return;
    if (file.size > 1_500_000) return setError('图片请控制在 1.5 MB 以内，以便保存在浏览器本地。');
    const avatar = await readFile(file);
    setDraft((current) => ({ ...current, avatar }));
  };
  return <Dialog title="编辑个人资料" onClose={onClose}><form onSubmit={(event) => { event.preventDefault(); onSave({ ...draft, tags: draft.tags.slice(0, 5) }); }}><div className="dialog-body form-grid"><label>姓名<input required value={draft.name} onChange={(event) => setDraft({ ...draft, name: event.target.value })} /></label><label>身份描述<input value={draft.headline} onChange={(event) => setDraft({ ...draft, headline: event.target.value })} /></label><label className="wide">个人简介<textarea value={draft.bio} onChange={(event) => setDraft({ ...draft, bio: event.target.value })} /></label><label className="wide">主题标签 <span className="field-hint">用逗号分隔，最多 5 个</span><input value={draft.tags.join(', ')} onChange={(event) => setDraft({ ...draft, tags: event.target.value.split(',').map((tag) => tag.trim()).filter(Boolean) })} /></label><label className="wide upload-field">头像图片<input type="file" accept="image/*" onChange={uploadAvatar} /></label>{error && <p className="form-error" role="alert">{error}</p>}</div><DialogFooter onClose={onClose} /></form></Dialog>;
}

function ContactChoiceModal({ onChoose, onClose }) {
  return <Dialog title="添加媒体与联系方式" onClose={onClose}><div className="dialog-body"><p className="form-intro">选择要展示的公开联系方式。模板只保存用户主动填写的链接或邮箱，不抓取平台内容。</p><div className="social-grid">{contactPlatforms.map((platform) => <button key={platform} type="button" onClick={() => onChoose(platform)}><span className="platform-mark">{platform === 'X / Twitter' ? 'X' : platform === 'GitHub' ? 'GH' : platform.slice(0, 1)}</span><span>{platform}</span></button>)}</div></div><div className="dialog-footer"><button className="secondary-button" type="button" onClick={onClose}>取消</button></div></Dialog>;
}

function LinkPreviewModal({ block, onClose }) {
  const definition = blockDefinitions.find((item) => item.type === block.type) ?? blockDefinitions[1];
  const title = block.type === 'contact' ? block.name || `${block.platform} 主页` : block.title || definition.label;
  const label = block.type === 'contact' ? block.platform : definition.label;
  const openLink = (event) => {
    if (!block.workSlug || typeof window.viceme?.navigation?.openWork !== 'function') return;
    event.preventDefault();
    window.viceme.navigation.openWork(block.workSlug).catch(() => {});
  };
  return <Dialog title="打开链接" onClose={onClose}>
    <div className="link-preview">
      <div className="preview-symbol"><Icon name={definition.icon} size={26} /></div>
      <p className="preview-kind">{label}</p>
      <h3>{title}</h3>
      <p className="preview-url">{block.url}</p>
      <a className="open-link" href={block.url} target="_blank" rel="noreferrer" onClick={openLink}>打开链接<Icon name="external" size={18} /></a>
      <p className="preview-caption">将在新标签页中打开</p>
    </div>
  </Dialog>;
}

function BlockModal({ block, onSave, onDelete, onClose }) {
  const [draft, setDraft] = useState(block);
  const [error, setError] = useState('');
  const update = (key, value) => setDraft((current) => ({ ...current, [key]: value }));
  const uploadCover = async (event) => {
    const file = event.target.files?.[0];
    if (!file) return;
    if (file.size > 1_500_000) return setError('图片请控制在 1.5 MB 以内，以便保存在浏览器本地。');
    update('cover', await readFile(file));
  };
  const submit = (event) => {
    event.preventDefault();
    if (!draft.url.trim()) return setError('请填写公开链接或邮箱。');
    if (draft.type === 'contact') {
      const contactUrl = draft.platform === '邮箱'
        ? `mailto:${draft.url.trim().replace(/^mailto:/, '')}`
        : draft.url.trim();
      if (draft.platform === '邮箱' && !/^mailto:[^@\s]+@[^@\s]+\.[^@\s]+$/i.test(contactUrl)) {
        return setError('请填写有效的邮箱地址。');
      }
      if (draft.platform !== '邮箱') {
        try {
          const parsed = new URL(contactUrl);
          if (!['http:', 'https:'].includes(parsed.protocol)) throw new Error();
        } catch {
          return setError('请填写以 http:// 或 https:// 开头的公开链接。');
        }
      }
      return onSave({
        ...draft,
        url: contactUrl,
        name: draft.name.trim() || displayNameFromUrl(contactUrl, draft.platform) || `${draft.platform} 主页`,
      });
    }
    onSave(draft);
  };
  const title = block.type === 'work' ? '补充作品信息' : `添加 ${block.platform}`;
  return <Dialog title={title} onClose={onClose}><form onSubmit={submit}><div className="dialog-body form-grid">{block.type === 'work' ? <><p className="form-intro wide">从链接导入作品后，标题、简介和封面均由你最终确认。没有可靠的自动抓取也不会阻断保存。</p><label className="wide">作品链接<input required type="url" placeholder="https://..." value={draft.url} onChange={(event) => update('url', event.target.value)} /></label><label className="wide">作品名称<input required value={draft.title} onChange={(event) => update('title', event.target.value)} /></label><label className="wide">作品简介<textarea placeholder="用一句话介绍这个作品" value={draft.description} onChange={(event) => update('description', event.target.value)} /></label><label className="wide upload-field">作品封面<input type="file" accept="image/*" onChange={uploadCover} /></label></> : <><p className="form-intro wide">填写你主动公开的联系方式。飞书、X 和 GitHub 使用主页链接；邮箱会保存为邮件链接。</p><label className="wide">{draft.platform === '邮箱' ? '邮箱地址' : '主页链接'}<input required type="text" placeholder={draft.platform === '邮箱' ? 'name@example.com' : 'https://...'} value={draft.platform === '邮箱' ? draft.url.replace(/^mailto:/, '') : draft.url} onChange={(event) => update('url', event.target.value)} /></label><label className="wide">展示名称<input placeholder={draft.platform === '邮箱' ? '工作邮箱' : '你的账号名称'} value={draft.name} onChange={(event) => update('name', event.target.value)} /></label></>}</div>{error && <p className="form-error" role="alert">{error}</p>}<DialogFooter onClose={onClose} onDelete={onDelete} /></form></Dialog>;
}

function DialogFooter({ onClose, onDelete }) {
  return <div className="dialog-footer">{onDelete ? <button className="delete-button" type="button" onClick={onDelete}><Icon name="trash" size={17} />删除</button> : <span />}<div><button className="secondary-button" type="button" onClick={onClose}>取消</button><button className="primary-button save-button" type="submit">保存 Block</button></div></div>;
}

export default App;
