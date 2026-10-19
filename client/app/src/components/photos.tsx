"use client";

import { useEffect, useState } from "react";
import { ImageViewer } from "antd-mobile";
import { fetchObjectURL } from "@/lib/api";

/** 照片都是原始记录的附件（OSS），要带令牌读；同一张图在一次打开 App 期间只取一次。 */
const cache = new Map<string, Promise<string>>();

function photoURL(noteKey: string, index: number): Promise<string> {
  const path = `/notes/${noteKey}/attachments/${index}`;
  let url = cache.get(path);
  if (!url) {
    url = fetchObjectURL(path);
    url.catch(() => cache.delete(path));
    cache.set(path, url);
  }
  return url;
}

export interface PhotoRef {
  noteKey: string;
  index: number;
  caption?: string;
}

/** 一排照片缩略图，点开看大图。size=sm 用在聊天气泡里。 */
export function PhotoStrip({ photos, size = "md", align = "start" }: { photos: PhotoRef[]; size?: "sm" | "md"; align?: "start" | "end" }) {
  const [urls, setUrls] = useState<(string | null)[]>([]);
  const [preview, setPreview] = useState<number | null>(null);
  const signature = photos.map((p) => `${p.noteKey}/${p.index}`).join(",");

  useEffect(() => {
    let alive = true;
    setUrls(photos.map(() => null));
    photos.forEach((photo, position) => {
      photoURL(photo.noteKey, photo.index)
        .then((url) => alive && setUrls((list) => list.map((value, i) => (i === position ? url : value))))
        .catch(() => {});
    });
    return () => {
      alive = false;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [signature]);

  if (!photos.length) return null;
  const loaded = urls.filter((url): url is string => !!url);
  return (
    <div className="photo-strip" data-size={size} data-align={align}>
      {photos.map((photo, position) => (
        <figure key={`${photo.noteKey}/${photo.index}`} className="photo-item">
          <div className="photo-frame" onClick={() => urls[position] && setPreview(loaded.indexOf(urls[position]!))}>
            {urls[position] ? <img src={urls[position]!} alt={photo.caption ?? ""} /> : <span className="photo-placeholder" />}
          </div>
          {photo.caption && <figcaption>{photo.caption}</figcaption>}
        </figure>
      ))}
      <ImageViewer.Multi getContainer={() => document.body} images={loaded} visible={preview !== null} defaultIndex={preview ?? 0} onClose={() => setPreview(null)} />
      <style jsx global>{`
        .photo-strip { display: flex; gap: 8px; overflow-x: auto; scrollbar-width: none; margin-top: 10px; }
        .photo-strip::-webkit-scrollbar { display: none; }
        .photo-strip[data-align="end"] { justify-content: flex-end; flex-wrap: wrap; margin: 0 0 6px; }
        .photo-item { margin: 0; flex-shrink: 0; width: 128px; }
        .photo-strip[data-size="sm"] .photo-item { width: 76px; }
        .photo-frame {
          width: 100%;
          aspect-ratio: 1;
          border-radius: 14px;
          overflow: hidden;
          background: var(--pa-fill);
          border: 0.5px solid var(--pa-line);
          cursor: zoom-in;
        }
        .photo-frame img { width: 100%; height: 100%; object-fit: cover; display: block; }
        .photo-placeholder { display: block; width: 100%; height: 100%; animation: pa-pulse 1.4s ease-in-out infinite; background: var(--pa-fill-strong); }
        .photo-item figcaption {
          margin-top: 4px;
          font-size: 11.5px;
          line-height: 1.4;
          color: var(--pa-text-3);
          display: -webkit-box;
          -webkit-line-clamp: 2;
          -webkit-box-orient: vertical;
          overflow: hidden;
        }
        @keyframes pa-pulse { 50% { opacity: 0.5; } }
      `}</style>
    </div>
  );
}

/** 选好、还没发出去的照片：本机预览，可移除。 */
export function PickedPhotos({ urls, onRemove }: { urls: string[]; onRemove: (index: number) => void }) {
  const [preview, setPreview] = useState<number | null>(null);
  if (!urls.length) return null;
  return (
    <div className="picked-photos">
      {urls.map((url, index) => (
        <div key={url} className="picked-photo">
          <img src={url} alt="" onClick={() => setPreview(index)} />
          <button type="button" aria-label="移除" onClick={() => onRemove(index)}>×</button>
        </div>
      ))}
      <ImageViewer.Multi getContainer={() => document.body} images={urls} visible={preview !== null} defaultIndex={preview ?? 0} onClose={() => setPreview(null)} />
      <style jsx global>{`
        .picked-photos { display: flex; gap: 8px; overflow-x: auto; padding: 2px 4px 8px; scrollbar-width: none; }
        .picked-photos::-webkit-scrollbar { display: none; }
        .picked-photo { position: relative; flex-shrink: 0; width: 64px; height: 64px; border-radius: 12px; overflow: hidden; background: var(--pa-fill); }
        .picked-photo img { width: 100%; height: 100%; object-fit: cover; }
        .picked-photo button {
          position: absolute;
          top: 3px;
          right: 3px;
          width: 20px;
          height: 20px;
          border-radius: 10px;
          border: 0;
          padding: 0;
          line-height: 20px;
          font-size: 15px;
          background: rgba(0, 0, 0, 0.55);
          color: #fff;
          cursor: pointer;
        }
      `}</style>
    </div>
  );
}
