import Image from "next/image";

/**
 * The Flaqo brand mark.
 *
 * The supplied artwork is raster, so it is rendered through next/image
 * for correct sizing and optimisation. The mark is used as supplied:
 * the flag already carries the brand gradient, so it is not placed on a
 * tile or given a background.
 *
 * The artwork is 283x260, so the width follows the height exactly and
 * the flag is never distorted. The rendered size is set in CSS; the
 * props here only describe the intrinsic ratio.
 */
export function BrandMark({
  priority = false,
}: {
  priority?: boolean;
}) {
  return (
    <Image
      className="brand-mark"
      src="/brand/flaqo-flag-128.png"
      alt=""
      width={283}
      height={260}
      priority={priority}
      aria-hidden="true"
    />
  );
}

/**
 * The brand mark with the wordmark beside it.
 *
 * The wordmark uses the supplied lettering artwork rather than a text
 * label, so the brand reads exactly as it was designed.
 */
export function BrandLockup({ priority = false }: { priority?: boolean }) {
  return (
    <span className="row" style={{ gap: "var(--space-3)" }}>
      <BrandMark priority={priority} />
      <Image
        className="brand-wordmark-image"
        src="/brand/flaqo-wordmark-art.png"
        alt="Flaqo"
        width={384}
        height={140}
        priority={priority}
      />
    </span>
  );
}
