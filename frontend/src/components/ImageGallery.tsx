"use client";

import { useState } from "react";
import type { AuctionImage } from "@/lib/types";

export function ImageGallery({ images, alt }: { images: AuctionImage[]; alt: string }) {
  const [index, setIndex] = useState(0);
  const current = images[Math.min(index, images.length - 1)];
  if (!current) {
    return <div className="flex aspect-square items-center justify-center rounded-xl bg-gray-100 text-6xl">🦎</div>;
  }
  return (
    <div className="space-y-2">
      <div className="aspect-square overflow-hidden rounded-xl bg-gray-100">
        {/* eslint-disable-next-line @next/next/no-img-element -- backend-resized images */}
        <img src={current.url} alt={alt} className="h-full w-full object-contain" />
      </div>
      {images.length > 1 && (
        <div className="grid grid-cols-4 gap-2">
          {images.map((img, i) => (
            <button
              key={img.id}
              onClick={() => setIndex(i)}
              aria-label={`圖片 ${i + 1}`}
              className={`aspect-square overflow-hidden rounded-lg border-2 ${i === index ? "border-emerald-600" : "border-transparent"}`}
            >
              {/* eslint-disable-next-line @next/next/no-img-element -- thumbnails */}
              <img src={img.url} alt="" className="h-full w-full object-cover" />
            </button>
          ))}
        </div>
      )}
    </div>
  );
}
