"use client";

interface AudioVisualizerProps {
  level: number;
  active: boolean;
}

export function AudioVisualizer({ level, active }: AudioVisualizerProps) {
  const bars = 24;
  const normalizedLevel = Math.min(level * 3, 1);

  return (
    <div className="flex items-end justify-center gap-[3px] h-16">
      {Array.from({ length: bars }).map((_, i) => {
        const distance = Math.abs(i - bars / 2) / (bars / 2);
        const heightFactor = active
          ? Math.max(0.1, normalizedLevel * (1 - distance * 0.6))
          : 0.05;
        const height = Math.max(4, heightFactor * 64);

        return (
          <div
            key={i}
            className="w-[3px] rounded-full transition-all duration-75"
            style={{
              height: `${height}px`,
              backgroundColor: active
                ? `rgba(99, 102, 241, ${0.4 + normalizedLevel * 0.6})`
                : "rgba(113, 113, 122, 0.3)",
            }}
          />
        );
      })}
    </div>
  );
}
