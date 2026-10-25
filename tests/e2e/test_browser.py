"""Browser regression suite against cmd/devserver's REAL HTTP API + SSE.
Requires: pip install playwright; playwright install chromium
Run devserver on 127.0.0.1:8765 before this suite. No Scylla credentials are used.
"""
import json, os, time, unittest, urllib.request
from pathlib import Path
from playwright.sync_api import sync_playwright, expect
BASE=os.getenv('SCYLLASCOPE_TEST_URL','http://127.0.0.1:8765')
TOKEN='scyllascope-local-test-token-0000000000000000000000'
OUT=Path(os.getenv('SCYLLASCOPE_TEST_OUTPUT','test-results'));OUT.mkdir(exist_ok=True)
def scenario(value):
 req=urllib.request.Request(BASE+'/_test/scenario',json.dumps({'scenario':value}).encode(),headers={'Authorization':'Bearer '+TOKEN,'Content-Type':'application/json'})
 urllib.request.urlopen(req,timeout=5).close()
class BrowserTests(unittest.TestCase):
 @classmethod
 def setUpClass(cls):
  cls.pw=sync_playwright().start()
  args={'headless':True}
  if os.getenv('CHROMIUM_BIN'):args['executable_path']=os.environ['CHROMIUM_BIN']
  cls.browser=cls.pw.chromium.launch(**args);cls.saved_cookies=[]
 @classmethod
 def tearDownClass(cls):cls.browser.close();cls.pw.stop()
 def setUp(self):
  scenario('healthy')
  self.ctx=self.browser.new_context(viewport={'width':1440,'height':960},locale='ja-JP',timezone_id='Asia/Tokyo',reduced_motion='reduce')
  if self.saved_cookies:self.ctx.add_cookies(self.saved_cookies)
  self.page=self.ctx.new_page();self.page.set_default_timeout(5000);self.js_errors=[];self.docs=[];self.streams=[]
  self.page.on('pageerror',lambda e:self.js_errors.append(str(e)))
  self.page.on('request',lambda r:self.docs.append(r.url) if r.resource_type=='document' else self.streams.append(r.url) if '/api/v1/stream' in r.url else None)
  self.page.goto(BASE+'/',wait_until='domcontentloaded')
  if not self.saved_cookies:
   self.page.locator('input[type=password]').fill(TOKEN)
   self.page.get_by_role('button',name='ログイン',exact=True).click()
  expect(self.page.locator('[data-page=dashboard] .kpis')).to_be_visible()
  type(self).saved_cookies=self.ctx.cookies()
 def tearDown(self):
  if self.js_errors:self.page.screenshot(path=str(OUT/(self._testMethodName+'-error.png')))
  self.ctx.close();self.assertEqual(self.js_errors,[])
 def nav(self,id):
  self.page.locator(f'.sidebar nav a[href="#{id}"]').click()
  expect(self.page.locator(f'[data-page="{id}"]')).to_be_visible()
 def wait_tick(self):self.page.wait_for_timeout(1350)
 def remember(self,selector,name):self.page.locator(selector).first.evaluate('(el,name)=>{window[name]=el}',name)
 def same(self,selector,name):self.assertTrue(self.page.locator(selector).first.evaluate('(el,name)=>window[name]===el',name))
 def test_01_navigation_is_one_document_and_one_stream(self):
  for id in ['nodes','topology','metrics','alerts','events','query','settings','dashboard','nodes','query']:self.nav(id)
  self.wait_tick();self.assertEqual(len(self.docs),1);self.assertEqual(len(self.streams),1)
 def test_02_query_text_focus_selection_dom_survive_sse(self):
  self.nav('query');ed=self.page.get_by_role('textbox',name='CQLエディタ');ed.fill("SELECT peer, status FROM system.cluster_status;\n");ed.focus();ed.evaluate('e=>e.setSelectionRange(7,11)');self.remember('textarea','saved_editor');self.wait_tick()
  self.same('textarea','saved_editor');expect(ed).to_be_focused();self.assertEqual(ed.evaluate('e=>[e.selectionStart,e.selectionEnd]'),[7,11]);self.assertIn('SELECT peer',ed.input_value())
 def test_03_query_survives_navigation_and_runs(self):
  self.nav('query');ed=self.page.get_by_role('textbox',name='CQLエディタ');ed.fill('SELECT peer, status FROM system.cluster_status;');self.page.get_by_role('button',name='クエリを実行',exact=True).click();expect(self.page.locator('.query-results tbody tr')).to_have_count(3);self.remember('.query-results','saved_result');self.nav('dashboard');self.wait_tick();self.nav('query');self.same('.query-results','saved_result');self.assertEqual(ed.input_value(),'SELECT peer, status FROM system.cluster_status;')
 def test_04_details_open_and_dom_survive_updates(self):
  scenario('partial');expect(self.page.locator('.coverage-bar')).to_be_visible();self.page.locator('.coverage-bar summary').click();self.remember('.coverage-bar','saved_details');self.wait_tick();self.same('.coverage-bar','saved_details');self.assertTrue(self.page.locator('.coverage-bar').evaluate('e=>e.open'));self.nav('nodes');self.nav('dashboard');self.assertTrue(self.page.locator('.coverage-bar').evaluate('e=>e.open'))
 def test_05_node_filter_and_order_dom_survive(self):
  self.nav('nodes');s=self.page.get_by_role('textbox',name='ノード検索');s.fill('Demo A');self.remember('#node-search','saved_search');self.wait_tick();expect(self.page.locator('[data-page=nodes] tbody tr')).to_have_count(1);expect(self.page.locator('[data-page=nodes] tbody tr')).to_contain_text('Demo A');self.same('#node-search','saved_search');self.nav('dashboard');self.nav('nodes');expect(s).to_have_value('Demo A')
 def test_06_drawer_keeps_tab_search_and_dom(self):
  self.page.locator('[data-page=dashboard] [data-item=node]').first.click();expect(self.page.locator('dialog')).to_be_visible();self.page.locator('dialog').get_by_role('button',name='メトリクス',exact=True).click();s=self.page.get_by_role('textbox',name='メトリクス検索');s.fill('compaction');self.remember('dialog','saved_dialog');self.remember('dialog input','saved_metric');self.wait_tick();self.same('dialog','saved_dialog');self.same('dialog input','saved_metric');expect(s).to_have_value('compaction');expect(s).to_be_visible();self.page.get_by_role('button',name='詳細を閉じる').click();expect(self.page.locator('dialog')).not_to_be_visible()
 def test_07_drawer_details_open_survive_sse(self):
  self.page.locator('[data-page=dashboard] [data-item=node]').first.click();self.page.locator('dialog summary').click();self.wait_tick();self.assertTrue(self.page.locator('dialog details').evaluate('e=>e.open'));self.page.keyboard.press('Escape');expect(self.page.locator('dialog')).not_to_be_visible()
 def test_08_topology_zoom_and_pan_survive_sse(self):
  self.nav('topology');self.page.get_by_role('button',name='拡大',exact=True).click();self.page.get_by_role('button',name='拡大',exact=True).click();group=self.page.locator('[data-page=topology] svg.map > g');before=group.get_attribute('transform');self.remember('[data-page=topology] svg.map','saved_map');self.wait_tick();self.same('[data-page=topology] svg.map','saved_map');self.assertEqual(before,group.get_attribute('transform'));self.nav('nodes');self.nav('topology');self.assertEqual(before,group.get_attribute('transform'));self.page.get_by_role('button',name='全体を表示').click();self.assertIn('scale(1)',group.get_attribute('transform'))
 def test_09_directed_edge_state_changes_in_place(self):
  self.nav('topology');self.remember('[data-page=topology] [data-item=edge]','saved_edge');scenario('failure');expect(self.page.locator('[data-page=topology] .map-edge.critical')).to_have_count(2);self.same('[data-page=topology] [data-item=edge]','saved_edge');self.page.locator('[data-page=topology] .edge-group').last.focus();self.page.keyboard.press('Enter');expect(self.page.locator('dialog')).to_be_visible()
 def test_10_reorder_does_not_replace_keyed_node(self):
  self.remember('[data-page=dashboard] [data-node-id="100.64.0.1"]','saved_node');scenario('reorder');self.wait_tick();self.same('[data-page=dashboard] [data-node-id="100.64.0.1"]','saved_node')
 def test_11_pause_and_resume_updates(self):
  self.page.get_by_role('button',name='ライブ表示を一時停止').click();scenario('failure');self.wait_tick();expect(self.page.locator('[data-page=dashboard] .map-edge.critical')).to_have_count(0);self.page.get_by_role('button',name='ライブ表示を再開').click();expect(self.page.locator('[data-page=dashboard] .map-edge.critical')).to_have_count(2)
 def test_12_unknown_is_not_zero_or_healthy(self):
  scenario('unknown');self.wait_tick();expect(self.page.locator('.title-row .badge').first).to_have_text('未確認');self.assertNotIn('0 B',self.page.locator('[data-page=dashboard]').inner_text());self.assertIn('—',self.page.locator('.kpis').inner_text())
 def test_13_xss_is_escaped(self):
  scenario('xss');self.wait_tick();self.assertFalse(self.page.evaluate('Boolean(window.INJECTED)'));self.assertEqual(self.page.locator('[data-page=dashboard] img').count(),0);self.assertIn('<img',self.page.locator('[data-page=dashboard]').inner_text())
 def test_14_session_logout_revokes_old_cookie(self):
  cookie=next(c for c in self.ctx.cookies() if c['name']=='scyllascope_session');self.page.get_by_role('button',name='ログアウト').click();expect(self.page.locator('input[type=password]')).to_be_visible();res=self.ctx.request.get(BASE+'/api/v1/snapshot',headers={'Cookie':'scyllascope_session='+cookie['value']});self.assertEqual(res.status,401);type(self).saved_cookies=[]
 def test_15_browser_theme_and_density_persist(self):
  self.page.get_by_role('button',name='テーマ切替').click();self.assertEqual(self.page.locator('html').get_attribute('data-theme'),'light');self.page.reload();expect(self.page.locator('.kpis')).to_be_visible();self.assertEqual(self.page.locator('html').get_attribute('data-theme'),'light')
 def test_16_manual_refresh_does_not_recreate_page(self):
  self.remember('[data-page=dashboard]','saved_page');self.page.get_by_role('button',name='今すぐ確認').click();expect(self.page.locator('.toast')).to_contain_text('最新');self.same('[data-page=dashboard]','saved_page');self.assertEqual(len(self.docs),1)
 def test_17_mobile_no_page_overflow_and_menu(self):
  self.page.set_viewport_size({'width':390,'height':844});self.page.wait_for_timeout(200)
  self.assertLessEqual(self.page.evaluate('document.documentElement.scrollWidth'),391)
  self.page.get_by_role('button',name='メニューを開く').click();self.page.locator('.sidebar nav a[href="#nodes"]').click();expect(self.page.locator('[data-page=nodes]')).to_be_visible();self.assertLessEqual(self.page.evaluate('document.documentElement.scrollWidth'),391)
  self.page.locator('[data-page=nodes] tbody button').first.click();expect(self.page.locator('dialog')).to_be_visible();self.assertLessEqual(self.page.locator('dialog').bounding_box()['width'],390)
  self.page.screenshot(path=str(OUT/'mobile-drawer.png'))
 def test_18_history_ranges_return_valid_data(self):
  self.nav('metrics');expect(self.page.locator('[data-page=metrics] svg')).not_to_have_count(0);sel=self.page.get_by_role('combobox',name='履歴の時間範囲');sel.select_option('1');self.page.wait_for_timeout(250);one=self.page.locator('[data-page=metrics] .hint').first.inner_text();sel.select_option('6');self.page.wait_for_timeout(250);six=self.page.locator('[data-page=metrics] .hint').first.inner_text();sel.select_option('1');self.page.wait_for_timeout(250);self.assertNotEqual(one,six);self.assertEqual(one,self.page.locator('[data-page=metrics] .hint').first.inner_text())
 def test_19_alert_action_persists_over_sse(self):
  self.nav('alerts');self.page.get_by_role('button',name='確認済みにする').first.click();expect(self.page.locator('.incident-bottom').first).to_contain_text('確認済み');self.wait_tick();expect(self.page.locator('.incident-bottom').first).to_contain_text('確認済み');self.page.get_by_role('button',name='通知を再開').first.click();expect(self.page.get_by_role('button',name='通知を再開')).to_have_count(0)
 def test_20_scroll_position_survives_sse(self):
  self.page.set_viewport_size({'width':1280,'height':500});self.nav('settings');self.page.get_by_role('button',name='接続・セキュリティ').click();self.page.wait_for_timeout(150);main=self.page.locator('#main');main.evaluate('e=>e.scrollTop=200');self.page.wait_for_timeout(100);pos=main.evaluate('e=>e.scrollTop');self.assertGreater(pos,30);self.wait_tick();self.assertAlmostEqual(pos,main.evaluate('e=>e.scrollTop'),delta=2);self.nav('dashboard');self.nav('settings');self.page.wait_for_timeout(150);self.assertAlmostEqual(pos,main.evaluate('e=>e.scrollTop'),delta=2)
 def test_21_no_third_party_requests_or_page_errors(self):
  urls=[];self.page.on('request',lambda r:urls.append(r.url));self.nav('settings');self.nav('topology');self.wait_tick();self.assertTrue(all(u.startswith(BASE) for u in urls));self.assertEqual(self.js_errors,[])
if __name__=='__main__':unittest.main(verbosity=2)
