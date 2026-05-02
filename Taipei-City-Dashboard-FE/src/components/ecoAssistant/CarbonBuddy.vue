<!--
  小碳寶 (CarbonBuddy) — eco assistant avatar with animated state expressions.

  Pure inline SVG + CSS keyframes (acts like a GIF without needing an external file).

  Props:
    state:  "idle" | "thinking" | "searching" | "found" | "typing" | "offline"
    size:   px width/height of the round wrapper           (default 64)
    ring:   draw teal radial-gradient circle behind the face (default true)
    decor:  show decorations (thought bubbles / magnifier / sparkles / zzz) (default true)
    tinted: white body + teal accents — for placement on coloured backgrounds (default false)
-->
<script setup>
import { computed } from "vue";

const props = defineProps({
	state: {
		type: String,
		default: "idle",
		validator: (v) =>
			["idle", "thinking", "searching", "found", "typing", "offline"].includes(v),
	},
	size: { type: Number, default: 64 },
	ring: { type: Boolean, default: true },
	decor: { type: Boolean, default: true },
	tinted: { type: Boolean, default: false },
});

// Color palette swaps when placed on a coloured background.
const palette = computed(() => {
	if (props.tinted) {
		return {
			leafStem: "#00b8a9",
			leafBody: "#00b8a9",
			head: "#ffffff",
			ear: "rgba(255,255,255,0.85)",
			cheek: "#ff8fb1",
			eye: "#00b8a9",
			mouth: "#00b8a9",
		};
	}
	return {
		leafStem: "#3FA85F",
		leafBody: "#5DC07C",
		head: "#A6D9A8",
		ear: "#7CC589",
		cheek: "#FFD0DC",
		eye: "#173D2A",
		mouth: "#173D2A",
	};
});

// Eye / mouth variants per state — derived from the original 動作狀態.html design.
const eyeStyle = computed(() => {
	switch (props.state) {
		case "thinking": return "look-up";
		case "searching": return "look-side";
		case "found": return "sparkle";
		case "offline": return "closed";
		case "typing":
		case "idle":
		default: return "open";
	}
});

const mouthStyle = computed(() => {
	switch (props.state) {
		case "thinking":
		case "searching": return "small";
		case "found": return "wide";
		case "typing": return "open-talk";
		case "offline": return "small";
		case "idle":
		default: return "happy";
	}
});

// face takes up most of the wrapper; decorations need extra room.
// With ring (mask), keep a bit of margin so the ears don't kiss the edge.
const faceScale = computed(() => {
	if (props.decor) return 0.78;
	return props.ring ? 0.86 : 0.96;
});
</script>

<template>
	<div
		class="cb"
		:class="[`cb--${state}`, { 'cb--ring': ring, 'cb--no-decor': !decor }]"
		:style="{ width: `${size}px`, height: `${size}px` }"
	>
		<!-- Decorations layered behind/around the face -->
		<template v-if="decor">
			<!-- Thinking: thought-bubble trail -->
			<template v-if="state === 'thinking'">
				<span class="cb-thought cb-thought--1" />
				<span class="cb-thought cb-thought--2" />
				<span class="cb-thought cb-thought--3">
					<span class="cb-dot" />
					<span class="cb-dot" />
					<span class="cb-dot" />
				</span>
			</template>

			<!-- Searching: scan glow + sweeping magnifier + orbiting docs -->
			<template v-if="state === 'searching'">
				<span class="cb-scan-glow" />
				<span class="cb-orbit">
					<span class="cb-orbit-item cb-orbit-item--1">
						<svg viewBox="0 0 28 28">
							<rect x="4" y="3" width="20" height="22" rx="3" fill="#FFFFFF" stroke="#0077B6" stroke-width="2" />
							<line x1="8" y1="9" x2="20" y2="9" stroke="#0077B6" stroke-width="1.6" stroke-linecap="round" />
							<line x1="8" y1="13" x2="20" y2="13" stroke="#0077B6" stroke-width="1.6" stroke-linecap="round" />
							<line x1="8" y1="17" x2="16" y2="17" stroke="#0077B6" stroke-width="1.6" stroke-linecap="round" />
						</svg>
					</span>
					<span class="cb-orbit-item cb-orbit-item--2">
						<svg viewBox="0 0 28 28">
							<rect x="4" y="3" width="20" height="22" rx="3" fill="#FFFFFF" stroke="#00B8A9" stroke-width="2" />
							<line x1="8" y1="9" x2="20" y2="9" stroke="#00B8A9" stroke-width="1.6" stroke-linecap="round" />
							<line x1="8" y1="13" x2="20" y2="13" stroke="#00B8A9" stroke-width="1.6" stroke-linecap="round" />
							<line x1="8" y1="17" x2="16" y2="17" stroke="#00B8A9" stroke-width="1.6" stroke-linecap="round" />
						</svg>
					</span>
					<span class="cb-orbit-item cb-orbit-item--3">
						<svg viewBox="0 0 28 28">
							<rect x="4" y="3" width="20" height="22" rx="3" fill="#FFFFFF" stroke="#BDB2FF" stroke-width="2" />
							<line x1="8" y1="9" x2="20" y2="9" stroke="#BDB2FF" stroke-width="1.6" stroke-linecap="round" />
							<line x1="8" y1="13" x2="20" y2="13" stroke="#BDB2FF" stroke-width="1.6" stroke-linecap="round" />
							<line x1="8" y1="17" x2="16" y2="17" stroke="#BDB2FF" stroke-width="1.6" stroke-linecap="round" />
						</svg>
					</span>
				</span>
				<svg class="cb-magnifier" viewBox="0 0 64 64">
					<circle
						cx="26"
						cy="26"
						r="16"
						fill="rgba(255,255,255,0.4)"
						stroke="#0077B6"
						stroke-width="4"
					/>
					<line
						x1="38"
						y1="38"
						x2="54"
						y2="54"
						stroke="#0077B6"
						stroke-width="6"
						stroke-linecap="round"
					/>
				</svg>
			</template>

			<!-- Found: sparkles + glowing lightbulb above head -->
			<template v-if="state === 'found'">
				<span class="cb-spark cb-spark--1" />
				<span class="cb-spark cb-spark--2" />
				<span class="cb-spark cb-spark--3" />
				<span class="cb-spark cb-spark--4" />
				<svg class="cb-lightbulb" viewBox="0 0 48 48">
					<circle cx="24" cy="20" r="18" fill="#FFD166" opacity="0.25" />
					<path
						d="M24 6 C 16 6 12 12 12 18 C 12 23 15 26 17 29 L 17 33 L 31 33 L 31 29 C 33 26 36 23 36 18 C 36 12 32 6 24 6 Z"
						fill="#FFD166"
					/>
					<path
						d="M16 12 Q 20 8 24 8"
						stroke="#FFFFFF"
						stroke-width="2"
						stroke-linecap="round"
						fill="none"
						opacity="0.7"
					/>
					<rect x="17" y="34" width="14" height="3" rx="1" fill="#B8860B" />
					<rect x="18" y="38" width="12" height="3" rx="1" fill="#B8860B" />
					<path d="M21 42 L 27 42" stroke="#B8860B" stroke-width="2" stroke-linecap="round" />
				</svg>
			</template>

			<!-- Typing: floating speech-bubble with dots -->
			<template v-if="state === 'typing'">
				<span class="cb-typing-bubble">
					<span class="cb-dot" />
					<span class="cb-dot" />
					<span class="cb-dot" />
				</span>
			</template>

			<!-- Offline: sleepy zzz -->
			<template v-if="state === 'offline'">
				<span class="cb-zzz cb-zzz--1">z</span>
				<span class="cb-zzz cb-zzz--2">Z</span>
				<span class="cb-zzz cb-zzz--3">Z</span>
			</template>
		</template>

		<!-- Buddy face -->
		<svg
			class="cb-face"
			:style="{ width: `${faceScale * 100}%`, height: `${faceScale * 100}%` }"
			viewBox="11 22 298 298"
			preserveAspectRatio="xMidYMid meet"
		>
			<!-- leaf -->
			<g :class="{ 'cb-leaf-glow': state === 'found' }">
				<path
					d="M170 110 L 170 60"
					:stroke="palette.leafStem"
					stroke-width="5"
					stroke-linecap="round"
					fill="none"
				/>
				<path
					d="M170 60 C 170 18 214 -2 244 12 C 244 50 218 78 178 78 C 174 74 170 68 170 60 Z"
					:fill="palette.leafBody"
				/>
				<path
					d="M174 70 C 190 54 212 38 238 22"
					stroke="#FFFFFF"
					stroke-width="2.5"
					stroke-linecap="round"
					fill="none"
					opacity="0.7"
				/>
			</g>

			<!-- head + ears + cheeks -->
			<circle cx="160" cy="200" r="120" :fill="palette.head" />
			<circle cx="56" cy="250" r="20" :fill="palette.ear" />
			<circle cx="264" cy="250" r="20" :fill="palette.ear" />
			<circle
				cx="100"
				cy="218"
				r="14"
				:fill="state === 'found' ? '#FF8FB1' : palette.cheek"
				opacity="0.9"
			/>
			<circle
				cx="220"
				cy="218"
				r="14"
				:fill="state === 'found' ? '#FF8FB1' : palette.cheek"
				opacity="0.9"
			/>

			<!-- eyes -->
			<template v-if="eyeStyle === 'closed'">
				<path
					d="M115 204 Q 128 212 141 204"
					:stroke="palette.eye"
					stroke-width="4"
					stroke-linecap="round"
					fill="none"
				/>
				<path
					d="M179 204 Q 192 212 205 204"
					:stroke="palette.eye"
					stroke-width="4"
					stroke-linecap="round"
					fill="none"
				/>
			</template>
			<template v-else-if="eyeStyle === 'look-up'">
				<circle cx="128" cy="200" r="13" :fill="palette.eye" />
				<circle cx="192" cy="200" r="13" :fill="palette.eye" />
				<circle cx="128" cy="194" r="6" fill="#FFFFFF" />
				<circle cx="192" cy="194" r="6" fill="#FFFFFF" />
			</template>
			<template v-else-if="eyeStyle === 'look-side'">
				<circle cx="128" cy="204" r="13" :fill="palette.eye" />
				<circle cx="192" cy="204" r="13" :fill="palette.eye" />
				<circle cx="134" cy="200" r="5" fill="#FFFFFF" />
				<circle cx="198" cy="200" r="5" fill="#FFFFFF" />
			</template>
			<template v-else-if="eyeStyle === 'sparkle'">
				<circle cx="128" cy="204" r="13" :fill="palette.eye" />
				<circle cx="192" cy="204" r="13" :fill="palette.eye" />
				<circle cx="124" cy="198" r="5" fill="#FFFFFF" />
				<circle cx="188" cy="198" r="5" fill="#FFFFFF" />
				<circle cx="133" cy="208" r="2" fill="#FFFFFF" />
				<circle cx="197" cy="208" r="2" fill="#FFFFFF" />
			</template>
			<template v-else>
				<!-- open (default) -->
				<circle cx="128" cy="204" r="13" :fill="palette.eye" />
				<circle cx="192" cy="204" r="13" :fill="palette.eye" />
				<circle cx="124" cy="199" r="3.5" fill="#FFFFFF" />
				<circle cx="188" cy="199" r="3.5" fill="#FFFFFF" />
			</template>

			<!-- mouth -->
			<ellipse
				v-if="mouthStyle === 'open-talk'"
				cx="160"
				cy="234"
				rx="9"
				ry="6"
				:fill="palette.mouth"
			>
				<animate
					attributeName="ry"
					values="6;3;6"
					dur="0.6s"
					repeatCount="indefinite"
				/>
			</ellipse>
			<path
				v-else-if="mouthStyle === 'wide'"
				d="M138 228 Q 160 254 182 228 Q 160 244 138 228 Z"
				:fill="palette.mouth"
			/>
			<path
				v-else-if="mouthStyle === 'small'"
				d="M152 234 Q 160 240 168 234"
				:stroke="palette.mouth"
				stroke-width="3.5"
				stroke-linecap="round"
				fill="none"
			/>
			<path
				v-else
				d="M144 232 Q 160 246 176 232"
				:stroke="palette.mouth"
				stroke-width="4"
				stroke-linecap="round"
				fill="none"
			/>
		</svg>
	</div>
</template>

<style scoped lang="scss">
.cb {
	position: relative;
	display: inline-flex;
	align-items: center;
	justify-content: center;
	box-sizing: border-box;
	flex: none;

	&--ring {
		border-radius: 50%;
		background: radial-gradient(circle at 30% 28%, #a6e8e0 0%, #81d8d0 60%, #5ec4bb 100%);
		box-shadow:
			0 4px 12px rgba(0, 119, 182, 0.18),
			inset 0 -3px 10px rgba(0, 0, 0, 0.08);
		overflow: hidden;
	}

	&--ring::after {
		content: "";
		position: absolute;
		inset: 0;
		border-radius: inherit;
		background: radial-gradient(
			circle at 75% 90%,
			rgba(255, 255, 255, 0.18),
			transparent 50%
		);
		pointer-events: none;
	}

	&--offline.cb--ring {
		filter: grayscale(0.4);
	}
}

/* face floats gently — universal across states */
.cb-face {
	position: relative;
	z-index: 1;
	animation: cb-float 3s ease-in-out infinite;
}

.cb--found .cb-face {
	animation-duration: 1.6s;
}

.cb-leaf-glow {
	filter: drop-shadow(0 0 4px rgba(255, 209, 102, 0.85));
	animation: cb-leaf-pulse 1.6s ease-in-out infinite;
}

/* ====== Common dots used by thought-bubble + typing-bubble ====== */
.cb-dot {
	width: 22%;
	height: 22%;
	max-width: 6px;
	max-height: 6px;
	min-width: 3px;
	min-height: 3px;
	border-radius: 50%;
	background: #00b8a9;
	animation: cb-dot-bounce 1.4s ease-in-out infinite;
}
.cb-dot:nth-child(2) { animation-delay: 0.16s; }
.cb-dot:nth-child(3) { animation-delay: 0.32s; }

/* ====== Thinking ====== */
.cb-thought {
	position: absolute;
	background: #ffffff;
	border: 2px solid #81d8d0;
	border-radius: 50%;
	box-shadow: 0 2px 6px rgba(0, 119, 182, 0.18);
	z-index: 2;
}
.cb-thought--1 {
	width: 9%; height: 9%; min-width: 5px; min-height: 5px;
	top: 22%; right: 24%;
}
.cb-thought--2 {
	width: 14%; height: 14%; min-width: 8px; min-height: 8px;
	top: 10%; right: 12%;
}
.cb-thought--3 {
	width: 38%; height: 26%; min-width: 22px; min-height: 16px;
	top: -2%; right: -4%;
	border-radius: 999px;
	display: flex;
	align-items: center;
	justify-content: center;
	gap: 3px;
	animation: cb-float 3s ease-in-out infinite;
}

/* ====== Searching ====== */
.cb-scan-glow {
	position: absolute;
	top: 50%; left: 50%;
	width: 70%; height: 70%;
	transform: translate(-50%, -50%);
	border-radius: 50%;
	background: radial-gradient(circle, rgba(0, 119, 182, 0.28), transparent 60%);
	animation: cb-scan-glow 2s ease-in-out infinite;
	pointer-events: none;
}
.cb-magnifier {
	position: absolute;
	top: 8%;
	right: 4%;
	width: 36%;
	height: 36%;
	transform-origin: center;
	animation: cb-mag-sweep 4s ease-in-out infinite;
	z-index: 3;
}

.cb-orbit {
	position: absolute;
	top: 50%;
	left: 50%;
	width: 0;
	height: 0;
	z-index: 2;
}
.cb-orbit-item {
	position: absolute;
	top: -14%;
	left: -14%;
	width: 22%;
	height: 22%;
	min-width: 14px;
	min-height: 14px;
	animation: cb-orbit 4s linear infinite;

	svg { width: 100%; height: 100%; display: block; }
}
.cb-orbit-item--2 { animation-delay: -1.33s; }
.cb-orbit-item--3 { animation-delay: -2.66s; }

.cb-lightbulb {
	position: absolute;
	top: -12%;
	left: 50%;
	width: 26%;
	height: 26%;
	min-width: 18px;
	min-height: 18px;
	transform: translateX(-50%);
	animation: cb-float 2s ease-in-out infinite, cb-bulb-glow 1.6s ease-in-out infinite;
	z-index: 3;
}

/* ====== Found / sparkles ====== */
.cb-spark {
	position: absolute;
	width: 12%;
	height: 12%;
	min-width: 8px;
	min-height: 8px;
	background:
		linear-gradient(#ffd166, #ffd166) center/30% 100% no-repeat,
		linear-gradient(#ffd166, #ffd166) center/100% 30% no-repeat;
	animation: cb-spark-burst 1.6s ease-in-out infinite;
	z-index: 2;
}
.cb-spark--1 { top: 8%;  left: 16%; animation-delay: 0s; }
.cb-spark--2 { top: 22%; right: 12%; background-color: transparent; animation-delay: 0.4s;
	background-image:
		linear-gradient(#ff8fb1, #ff8fb1),
		linear-gradient(#ff8fb1, #ff8fb1);
	background-size: 30% 100%, 100% 30%;
	background-position: center, center;
	background-repeat: no-repeat, no-repeat;
}
.cb-spark--3 { bottom: 22%; left: 10%; animation-delay: 0.8s;
	background-image:
		linear-gradient(#bdb2ff, #bdb2ff),
		linear-gradient(#bdb2ff, #bdb2ff);
	background-size: 30% 100%, 100% 30%;
	background-position: center, center;
	background-repeat: no-repeat, no-repeat;
}
.cb-spark--4 { bottom: 12%; right: 18%; animation-delay: 1.2s; }

/* ====== Typing bubble ====== */
.cb-typing-bubble {
	position: absolute;
	top: 8%;
	right: -2%;
	background: #ffffff;
	border: 2px solid #81d8d0;
	border-radius: 14px;
	padding: 18% 12%;
	display: flex;
	gap: 3px;
	align-items: center;
	box-shadow: 0 2px 8px rgba(0, 119, 182, 0.15);
	z-index: 2;
}

/* ====== Offline / zzz ====== */
.cb-zzz {
	position: absolute;
	font-family: "Poppins", sans-serif;
	font-weight: 700;
	color: #5c7080;
	z-index: 2;
	animation: cb-float 3s ease-in-out infinite;
}
.cb-zzz--1 { top: 18%; right: 20%; font-size: 11%; opacity: 0.55; animation-delay: 0s; }
.cb-zzz--2 { top: 8%;  right: 10%; font-size: 16%; opacity: 0.75; animation-delay: -1s; }
.cb-zzz--3 { top: -4%; right: -2%; font-size: 22%; opacity: 0.9;  animation-delay: -2s; }

/* For tiny avatars (no decor), make sure font-size for zzz uses px not %  */
.cb--no-decor .cb-zzz { display: none; }

@keyframes cb-float {
	0%, 100% { transform: translateY(0); }
	50% { transform: translateY(-4%); }
}
@keyframes cb-leaf-pulse {
	0%, 100% { filter: drop-shadow(0 0 4px rgba(255, 209, 102, 0.6)); }
	50% { filter: drop-shadow(0 0 10px rgba(255, 209, 102, 1)); }
}
@keyframes cb-dot-bounce {
	0%, 80%, 100% { transform: translateY(0); opacity: 0.5; }
	40% { transform: translateY(-30%); opacity: 1; }
}
@keyframes cb-scan-glow {
	0%, 100% { opacity: 0.3; transform: translate(-50%, -50%) scale(1); }
	50% { opacity: 0.6; transform: translate(-50%, -50%) scale(1.15); }
}
@keyframes cb-mag-sweep {
	0%   { transform: rotate(-15deg) translate(-12%, -4%); }
	25%  { transform: rotate(8deg)  translate(8%, -8%); }
	50%  { transform: rotate(-6deg) translate(12%, 4%); }
	75%  { transform: rotate(12deg) translate(-4%, 6%); }
	100% { transform: rotate(-15deg) translate(-12%, -4%); }
}
@keyframes cb-spark-burst {
	0%, 100% { transform: scale(0) rotate(0deg); opacity: 0; }
	50% { transform: scale(1) rotate(45deg); opacity: 1; }
}
@keyframes cb-orbit {
	from { transform: rotate(0deg) translateX(140%) rotate(0deg); }
	to { transform: rotate(360deg) translateX(140%) rotate(-360deg); }
}
@keyframes cb-bulb-glow {
	0%, 100% { filter: drop-shadow(0 0 4px rgba(255, 209, 102, 0.6)); }
	50% { filter: drop-shadow(0 0 14px rgba(255, 209, 102, 1)); }
}
</style>