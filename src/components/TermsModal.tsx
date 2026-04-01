import React from 'react';
import ReactDOM from 'react-dom';
import './TermsModal.css';

interface TermsModalProps {
    show: boolean;
    onClose: () => void;
}

const TermsModal: React.FC<TermsModalProps> = ({ show, onClose }) => {
    if (!show) return null;

    return ReactDOM.createPortal(
        <div className="modal-overlay" onClick={onClose}>
            <div className="modal-content glass-card" onClick={e => e.stopPropagation()}>
                <div className="modal-header">
                    <h3>平台服務條款與競標規則</h3>
                    <button className="close-btn" onClick={onClose}>&times;</button>
                </div>
                <div className="modal-body">
                    <section>
                        <h4>1. 條款接受</h4>
                        <p>使用者在註冊帳號或使用本網站服務前，應詳細閱讀本使用條款。當使用者完成註冊或使用本網站服務，即視為已閱讀、理解並同意遵守本條款之所有內容。</p>
                    </section>
                    <section>
                        <h4>2. 帳號與資料</h4>
                        <p>使用者應提供真實且正確之資料進行註冊，並妥善保管帳號及密碼。因帳號管理不當所造成之損失，由使用者自行負責。</p>
                    </section>
                    <section>
                        <h4>3. 競標規則</h4>
                        <ul>
                            <li>1. 使用者可於競標期間內對商品進行出價，每次出價須高於目前最高出價並符合商品設定之最低加價幅度。</li>
                            <li>2. 使用者一旦提交出價，即視為具有購買意願之承諾。</li>
                            <li>3. 若在結標前1分鐘內出價，則結標時間自動延長兩分鐘，直到無人出價為止</li>
                            <li>4. 競標結束時，出價最高且符合規則之使用者即為得標者。</li>
                            <li>5. 得標者應於得標後24小時內完成付款或交易程序，否則視為棄標。</li>
                            <li>6. 有參與競標的得標紀錄保留24小時</li>
                        </ul>
                    </section>
                    <section>
                        <h4>4. 棄標與違規處理</h4>
                        <p>若得標者未依規定完成交易，本網站有權取消其得標資格，並可能採取以下措施：</p>
                        <ul>
                            <li>1. 限制或暫停帳號使用</li>
                            <li>2. 取消相關競標結果</li>
                            <li>3. 限制未來參與競標之權利</li>
                            <li>4. 若有「刪標、棄標」情況發生,將由版主決定是否由上一個出價者得標。</li>
                        </ul>
                    </section>
                    <section>
                        <h4>5. 交付方式</h4>
                        <ul>
                            <li>1. 7-11 +$60</li>
                            <li>2. 黑貓宅配 +$130</li>
                            <li>3. 頭份自取，平日13:00-17:00</li>
                        </ul>
                    </section>
                    <section>
                        <h4>6. 禁止行為</h4>
                        <p>使用者不得從事以下行為：</p>
                        <ul>
                            <li>1. 提供不實資料或冒用他人身分註冊帳號。</li>
                            <li>2. 使用多個帳號操控或干擾競標價格。</li>
                            <li>3. 使用自動化程式、機器人或其他技術手段影響競標公平性。</li>
                            <li>4. 利用系統漏洞、程式錯誤（Bug）或非正常方式獲取不當利益。</li>
                            <li>5. 以任何方式干擾或破壞網站系統正常運作。</li>
                            <li>6. 若發現上述行為，本網站有權取消競標結果並終止帳號使用權。</li>
                        </ul>
                    </section>
                    <section>
                        <h4>7. 系統與服務聲明</h4>
                        <p>本網站將盡力維持服務穩定，但不保證服務完全不中斷或無錯誤。</p>
                        <p>如因系統異常、網路延遲、技術問題或其他不可抗力因素導致競標資料異常，本網站有權取消或調整競標結果。</p>
                    </section>
                    <section>
                        <h4>8. 條款修改與解釋</h4>
                        <p>本網站有權隨時修改本條款內容並公告於網站。使用者繼續使用本服務即視為同意修改後條款。在法律允許範圍內，本網站保留本條款之最終解釋權。</p>
                    </section>
                </div>
                <div className="modal-footer">
                    <button className="btn-primary" onClick={onClose}>關閉</button>
                </div>
            </div>
        </div>,
        document.body
    );
};

export default TermsModal;
