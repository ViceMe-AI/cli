import React, { useEffect, useRef, useState } from 'react';
import { initialBlocks, initialProfile } from './data';

function Icon({ name, size = 20 }) {
  const common = { width: size, height: size, viewBox: '0 0 24 24', fill: 'none', stroke: 'currentColor', strokeWidth: 1.8, strokeLinecap: 'round', strokeLinejoin: 'round', 'aria-hidden': true };
  if (name === 'work') return <svg {...common}><rect x="4" y="4" width="16" height="16" rx="3" /><path d="M8 10h8M8 14h5" /></svg>;
  if (name === 'social') return <svg {...common}><path d="M10.4 13.6a3 3 0 0 0 4.2 0l2.3-2.3a3 3 0 1 0-4.2-4.2l-1.3 1.3" /><path d="M13.6 10.4a3 3 0 0 0-4.2 0l-2.3 2.3a3 3 0 1 0 4.2 4.2l1.3-1.3" /></svg>;
  if (name === 'close') return <svg {...common}><path d="m6 6 12 12M18 6 6 18" /></svg>;
  if (name === 'external') return <svg {...common}><path d="M14 4h6v6M20 4l-9 9" /><path d="M18 13v5a2 2 0 0 1-2 2H6a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h5" /></svg>;
  return null;
}

function App() {
  const [modal, setModal] = useState(null);

  return (
    <div className="app-shell">
      <header className="topbar">
        <a className="brand" href="#profile" aria-label="回到个人主页顶部"><span className="brand-mark"><span /></span> Bonjour Card</a>
      </header>

      <main className="workspace" id="profile">
        <ProfileRail profile={initialProfile} />
        <section className="canvas" aria-labelledby="canvas-title">
          <div className="canvas-heading">
            <div><h1 id="canvas-title">个人主页</h1><p className="canvas-copy">这里展示由 Agent 根据你确认的资料整理出的作品和公开联系方式。</p></div>
          </div>
          <div className="block-stack">
            {initialBlocks.map((block) => <BlockCard key={block.id} block={block} onOpen={setModal} />)}
          </div>
        </section>
      </main>

      {modal && <LinkPreviewModal block={modal} onClose={() => setModal(null)} />}
    </div>
  );
}

function ProfileRail({ profile }) {
  return <aside className="profile-rail" aria-label="个人资料">
    <div className={'avatar ' + (profile.avatar ? 'has-image' : '')} style={profile.avatar ? { backgroundImage: 'url(' + profile.avatar + ')' } : undefined} aria-label={profile.avatar ? '个人头像' : '头像占位'} />
    <h2>{profile.name}</h2><p className="headline">{profile.headline}</p><p className="bio">{profile.bio}</p>
    <div className="rail-rule" />
    <p className="section-label">主题标签</p><div className="tag-list">{profile.tags.filter(Boolean).map((tag) => <span key={tag}>{tag}</span>)}</div>
  </aside>;
}

function validatedLink(value) {
  if (typeof value !== 'string' || !value.trim()) return null;
  try {
    const url = new URL(value.trim());
    if (!['http:', 'https:', 'mailto:'].includes(url.protocol)) return null;
    if (url.protocol === 'mailto:' && !/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(url.pathname)) return null;
    return url.href;
  } catch {
    return null;
  }
}

function BlockCard({ block, onOpen }) {
  const isContact = block.type === 'contact';
  const title = isContact ? block.name || (block.platform + ' 主页') : block.title || '作品';
  const description = isContact ? block.url : block.description;
  const href = validatedLink(block.url);
  const canOpen = Boolean(href);

  return <article className={'block-card block-' + block.type}>
    {block.type === 'work' && block.cover ? <div className="block-cover" style={{ backgroundImage: 'url(' + block.cover + ')' }} aria-hidden="true" /> : <div className="block-symbol"><Icon name={isContact ? 'social' : 'work'} size={21} /></div>}
    {canOpen ? <button className="block-main" type="button" onClick={() => onOpen({ ...block, href })} aria-label={'查看 ' + title + ' 的链接'}><BlockCopy type={isContact ? block.platform : '作品'} title={title} description={description} /></button> : <div className="block-main"><BlockCopy type={isContact ? block.platform : '作品'} title={title} description={description} /></div>}
  </article>;
}

function BlockCopy({ type, title, description }) {
  return <><span className="block-type">{type}</span><strong>{title}</strong>{description && <span>{description}</span>}</>;
}

function Dialog({ title, children, onClose }) {
  const dialogRef = useRef(null);
  const lastFocused = useRef(document.activeElement);

  useEffect(() => {
    const dialog = dialogRef.current;
    const focusable = () => [...dialog.querySelectorAll('button:not([disabled]), a[href]')];
    focusable()[0]?.focus();

    const handleKeyDown = (event) => {
      if (event.key === 'Escape') onClose();
      if (event.key !== 'Tab') return;
      const controls = focusable();
      const first = controls[0];
      const last = controls.at(-1);
      if (!first || !last) return;
      if (event.shiftKey && document.activeElement === first) {
        event.preventDefault();
        last.focus();
      } else if (!event.shiftKey && document.activeElement === last) {
        event.preventDefault();
        first.focus();
      }
    };
    dialog.addEventListener('keydown', handleKeyDown);
    return () => {
      dialog.removeEventListener('keydown', handleKeyDown);
      lastFocused.current?.focus();
    };
  }, [onClose]);

  return <div className="dialog-backdrop" role="presentation" onMouseDown={(event) => { if (event.target === event.currentTarget) onClose(); }}><section className="dialog" ref={dialogRef} role="dialog" aria-modal="true" aria-labelledby="dialog-title"><header><h2 id="dialog-title">{title}</h2><button className="icon-button" type="button" onClick={onClose} aria-label="关闭"><Icon name="close" /></button></header>{children}</section></div>;
}

function LinkPreviewModal({ block, onClose }) {
  const isContact = block.type === 'contact';
  const title = isContact ? block.name || (block.platform + ' 主页') : block.title || '作品';
  const label = isContact ? block.platform : '作品';
  const openLink = (event) => {
    if (!block.workSlug || typeof window.viceme?.navigation?.openWork !== 'function') return;
    event.preventDefault();
    window.viceme.navigation.openWork(block.workSlug).catch(() => {});
  };

  return <Dialog title="打开链接" onClose={onClose}>
    <div className="link-preview">
      <div className="preview-symbol"><Icon name={isContact ? 'social' : 'work'} size={26} /></div>
      <p className="preview-kind">{label}</p>
      <h3>{title}</h3>
      <p className="preview-url">{block.url}</p>
      <a className="open-link" href={block.href} target="_blank" rel="noreferrer" onClick={openLink}>打开链接<Icon name="external" size={18} /></a>
      <p className="preview-caption">将在新标签页中打开</p>
    </div>
  </Dialog>;
}

export default App;
