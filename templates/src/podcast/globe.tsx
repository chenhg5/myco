// A small, self-contained discovery globe. Marks represent editorial regions.
import { useLayoutEffect, useRef, useState } from "react";
import * as THREE from "three";
import { OrbitControls } from "three/addons/controls/OrbitControls.js";
import { language } from "../_lib/i18n";
import "./globe.css";

export type GlobeRegion = { id: string; label: string; lat: number; lon: number };
type Props = { regions: readonly GlobeRegion[]; selected?: string | null; onSelect: (id: string) => void };

// Simplified, hand-drawn silhouettes for the globe artwork. These illustrate
// the continents without drawing political borders or defining channel locations.
const land: number[][][] = [
  [[-168,64],[-155,72],[-135,72],[-125,75],[-98,73],[-86,80],[-69,78],[-61,68],[-82,63],[-88,54],[-80,52],[-60,53],[-63,45],[-76,37],[-80,26],[-89,30],[-97,25],[-92,19],[-83,21],[-87,15],[-79,9],[-79,7],[-89,14],[-98,18],[-107,25],[-114,31],[-118,34],[-125,49],[-130,55],[-142,60],[-155,58],[-164,61]],
  [[-75,60],[-82,69],[-70,81],[-45,84],[-24,78],[-20,71],[-39,64],[-53,59],[-60,62]],
  [[-81,9],[-73,12],[-66,10],[-61,9],[-51,3],[-48,-2],[-36,-7],[-38,-15],[-48,-25],[-52,-31],[-62,-39],[-67,-53],[-74,-52],[-74,-43],[-71,-33],[-71,-24],[-76,-15],[-81,-6]],
  [[-11,36],[-5,36],[1,40],[10,43],[16,39],[22,36],[29,41],[36,43],[32,46],[28,46],[29,53],[20,58],[13,55],[8,53],[3,51],[-5,47],[-9,43]],
  [[-11,35],[-3,36],[10,37],[19,32],[33,31],[37,23],[44,13],[51,11],[46,3],[40,-5],[40,-15],[34,-24],[27,-34],[19,-35],[15,-29],[12,-20],[12,-10],[7,3],[-3,5],[-12,8],[-17,16],[-15,25]],
  [[34,30],[41,31],[51,26],[57,22],[53,16],[43,13],[41,18]],
  [[43,-13],[50,-13],[49,-22],[45,-26],[43,-22]],
  [[-10,51],[-10,55],[-6,56],[-5,52]],
  [[-6,50],[-4,55],[-7,58],[-4,59],[0,55],[2,52],[-1,50]],
  [[5,58],[6,63],[12,68],[17,71],[27,70],[30,67],[23,64],[20,57],[15,55],[12,58],[10,62]],
  [[22,58],[29,68],[41,70],[52,68],[62,70],[69,73],[89,77],[105,75],[110,72],[133,72],[143,75],[163,71],[174,69],[180,68],[180,64],[170,62],[160,62],[162,55],[155,50],[146,43],[141,46],[136,45],[129,41],[127,34],[121,40],[118,37],[122,30],[119,24],[109,21],[106,14],[109,10],[104,10],[102,3],[98,7],[97,16],[93,19],[92,22],[89,21],[85,17],[79,8],[74,11],[72,19],[68,23],[60,25],[57,29],[52,28],[48,30],[45,38],[36,41],[32,46],[28,48]],
  [[-180,68],[-171,67],[-170,64],[-180,64]],
  [[131,33],[133,34],[138,36],[142,41],[145,44],[146,39],[143,34],[137,33],[132,31]],
  [[122,24],[122,21],[120,22]],
  [[119,19],[122,18],[122,14],[125,11],[124,6],[121,11]],
  [[95,6],[100,4],[105,-3],[106,-6],[102,-5],[98,0]],
  [[106,-6],[114,-7],[116,-9],[108,-8]],
  [[110,7],[118,6],[120,1],[116,-5],[109,-3]],
  [[132,-2],[141,-2],[150,-7],[150,-10],[141,-8],[136,-5]],
  [[112,-22],[114,-17],[123,-15],[127,-14],[132,-12],[137,-12],[139,-17],[142,-10],[145,-15],[149,-21],[154,-27],[150,-35],[145,-39],[140,-38],[136,-34],[129,-32],[124,-34],[117,-35],[115,-31]],
  [[145,-41],[150,-40],[148,-44],[145,-44]],
  [[167,-46],[168,-43],[174,-40],[175,-43],[171,-46]],
  [[174,-40],[176,-37],[174,-35],[178,-36],[179,-39],[176,-41]],
  [[-180,-72],[-157,-75],[-125,-72],[-102,-73],[-77,-73],[-64,-65],[-55,-72],[-27,-76],[0,-72],[30,-70],[60,-67],[87,-66],[115,-66],[140,-70],[165,-71],[180,-72],[180,-90],[-180,-90]],
];

function artwork() {
  const map = document.createElement("canvas");
  map.width = 1536;
  map.height = 768;
  const ctx = map.getContext("2d");
  if (!ctx) throw new Error("Globe texture context is unavailable");
  const x = (lon: number) => (lon + 180) / 360 * map.width;
  const y = (lat: number) => (90 - lat) / 180 * map.height;
  ctx.fillStyle = "#263c43";
  ctx.fillRect(0, 0, map.width, map.height);
  for (const polygon of land) {
    ctx.beginPath();
    polygon.forEach(([lon, lat], index) => index ? ctx.lineTo(x(lon), y(lat)) : ctx.moveTo(x(lon), y(lat)));
    ctx.closePath();
    ctx.fillStyle = "#5c726e";
    ctx.fill();
    ctx.strokeStyle = "#81948a";
    ctx.lineWidth = 1.1;
    ctx.stroke();
  }

  ctx.beginPath();
  for (let lon = -180; lon < 180; lon += 15) { ctx.moveTo(x(lon), 0); ctx.lineTo(x(lon), map.height); }
  for (let lat = -75; lat <= 75; lat += 15) { ctx.moveTo(0, y(lat)); ctx.lineTo(map.width, y(lat)); }
  ctx.strokeStyle = "rgba(190,216,210,0.25)";
  ctx.lineWidth = 1;
  ctx.stroke();
  const texture = new THREE.CanvasTexture(map);
  texture.colorSpace = THREE.SRGBColorSpace;
  return texture;
}

function point(lat: number, lon: number) {
  const latitude = THREE.MathUtils.degToRad(lat);
  const longitude = THREE.MathUtils.degToRad(lon);
  return new THREE.Vector3(
    Math.cos(latitude) * Math.cos(longitude),
    Math.sin(latitude), -Math.cos(latitude) * Math.sin(longitude),
  );
}

function mountGlobe(
  canvas: HTMLCanvasElement,
  regions: () => readonly GlobeRegion[],
  buttons: Map<string, HTMLButtonElement>,
  onLost: () => void,
) {
  const renderer = new THREE.WebGLRenderer({ canvas, alpha: true, antialias: true });
  let texture: THREE.CanvasTexture;
  try { texture = artwork(); }
  catch (error) { renderer.dispose(); renderer.forceContextLoss(); throw error; }
  renderer.setPixelRatio(Math.min(window.devicePixelRatio || 1, 2));
  const scene = new THREE.Scene();
  const camera = new THREE.PerspectiveCamera(40, 1, 0.1, 20);
  camera.position.copy(point(18, 22).multiplyScalar(3.5));
  const geometry = new THREE.SphereGeometry(1, 72, 40);
  const material = new THREE.MeshStandardMaterial({ map: texture, roughness: 1 });
  scene.add(new THREE.Mesh(geometry, material));
  scene.add(new THREE.HemisphereLight(0xe9f2ed, 0xbbc4c4, 1.5));
  const sun = new THREE.DirectionalLight(0xfff0d8, 1.8);
  scene.add(sun);

  const controls = new OrbitControls(camera, canvas);
  controls.enablePan = false;
  controls.rotateSpeed = 0.65;
  controls.minDistance = 2.35;
  controls.maxDistance = 5.6;
  controls.minPolarAngle = 0.1;
  controls.maxPolarAngle = Math.PI - 0.1;

  const projected = new THREE.Vector3();
  const lightOffset = new THREE.Vector3(-1, 2, 1);
  let width = 0;
  let height = 0;
  function draw() {
    if (!width || !height) return;
    // Keep the light tied to the viewpoint as the user explores the globe.
    sun.position.copy(camera.position).add(lightOffset);
    renderer.render(scene, camera);
    for (const region of regions()) {
      const button = buttons.get(region.id);
      if (!button) continue;
      const surface = point(region.lat, region.lon);
      projected.copy(surface).project(camera);
      const visible = surface.dot(camera.position) > 1.04 &&
        Math.abs(projected.x) < 0.98 && Math.abs(projected.y) < 0.98;
      button.hidden = !visible;
      if (visible) {
        button.style.left = ((projected.x + 1) * width / 2) + "px";
        button.style.top = ((1 - projected.y) * height / 2) + "px";
      }
    }
  }
  function resize() {
    width = canvas.clientWidth;
    height = canvas.clientHeight;
    if (!width || !height) return;
    renderer.setSize(width, height, false);
    camera.aspect = width / height;
    camera.fov = THREE.MathUtils.radToDeg(2 * Math.atan(Math.tan(THREE.MathUtils.degToRad(20)) / Math.min(1, camera.aspect)));
    camera.updateProjectionMatrix();
    draw();
  }
  function keyboard(event: KeyboardEvent) {
    const position = new THREE.Spherical().setFromVector3(camera.position);
    switch (event.key) {
      case "ArrowLeft": position.theta -= 0.15; break;
      case "ArrowRight": position.theta += 0.15; break;
      case "ArrowUp": position.phi -= 0.15; break;
      case "ArrowDown": position.phi += 0.15; break;
      case "+": case "=": position.radius *= 0.9; break;
      case "-": position.radius /= 0.9; break;
      default: return;
    }
    event.preventDefault();
    position.makeSafe();
    camera.position.setFromSpherical(position);
    controls.update();
  }
  const lost = (event: Event) => { event.preventDefault(); onLost(); };
  controls.addEventListener("change", draw);
  canvas.addEventListener("keydown", keyboard);
  canvas.addEventListener("webglcontextlost", lost);
  const sized = new ResizeObserver(resize);
  sized.observe(canvas);
  resize();
  return {
    draw,
    dispose() {
      sized.disconnect();
      canvas.removeEventListener("keydown", keyboard);
      canvas.removeEventListener("webglcontextlost", lost);
      controls.removeEventListener("change", draw);
      controls.dispose();
      geometry.dispose();
      material.dispose();
      texture.dispose();
      renderer.dispose();
    },
  };
}

export function Globe({ regions, selected, onSelect }: Props) {
  const canvas = useRef<HTMLCanvasElement>(null);
  const markers = useRef(new Map<string, HTMLButtonElement>());
  const current = useRef(regions);
  const view = useRef<ReturnType<typeof mountGlobe>>();
  const [fallback, setFallback] = useState(false);
  const zh = language() === "zh";
  const label = zh ? "选择播客地区" : "Choose a podcast region";
  const globeLabel = zh ? "播客地球，可拖动旋转或缩放" : "Podcast globe. Drag to rotate or zoom.";

  useLayoutEffect(() => {
    if (fallback || !canvas.current) return;
    try {
      view.current = mountGlobe(canvas.current, () => current.current, markers.current, () => setFallback(true));
    } catch { setFallback(true); }
    return () => {
      view.current?.dispose();
      view.current = undefined;
    };
  }, [fallback]);

  useLayoutEffect(() => {
    current.current = regions;
    view.current?.draw();
  }, [regions]);

  return <div className="pg-globe">
    {fallback ? <div className="pg-globe-fallback">
      <p>{label}</p>
      <div className="pg-globe-regions" role="group" aria-label={label}>
        {regions.map(region => <button type="button" key={region.id}
          className="btn" aria-pressed={selected === region.id} onClick={() => onSelect(region.id)}>{region.label}</button>)}
      </div>
    </div> : <>
      <canvas ref={canvas} className="pg-globe-canvas" tabIndex={0} aria-label={globeLabel}
        aria-keyshortcuts="ArrowLeft ArrowRight ArrowUp ArrowDown + -"/>
      <div className="pg-globe-marks" role="group" aria-label={label}>
        {regions.map(region => <button ref={node => {
          if (node) markers.current.set(region.id, node);
          else markers.current.delete(region.id);
        }} hidden className="pg-globe-marker" type="button" key={region.id}
          aria-label={region.label} aria-pressed={selected === region.id}
          onClick={event => { if (!event.currentTarget.hidden) onSelect(region.id); }}>
          <span aria-hidden="true" className="pg-globe-dot"/>
          <span aria-hidden="true" className="pg-globe-label">{region.label}</span>
        </button>)}
      </div>
    </>}
  </div>;
}
